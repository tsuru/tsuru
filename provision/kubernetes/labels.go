// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/set"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	tsuruLabelPrefix = "tsuru.io/"
)

type labelSet struct {
	labels      map[string]string
	annotations map[string]string
}

func withPrefix(m map[string]string) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		if !strings.HasPrefix(k, tsuruLabelPrefix) {
			k = tsuruLabelPrefix + k
		}
		result[k] = v
	}
	return result
}

func subMap(m map[string]string, keys ...string) map[string]string {
	result := make(map[string]string, len(keys))
	s := set.FromValues(keys...)
	for k, v := range m {
		if s.Includes(k) {
			result[k] = v
		}
	}
	return result
}

func (s *labelSet) ToLabels() map[string]string {
	return withPrefix(s.labels)
}

func (s *labelSet) ToAnnotations() map[string]string {
	return withPrefix(s.annotations)
}

func (s *labelSet) ToSelector() map[string]string {
	return withPrefix(subMap(s.labels, "app-name", "app-process", "is-build"))
}

func (s *labelSet) ToAppSelector() map[string]string {
	return withPrefix(subMap(s.labels, "app-name"))
}

func (s *labelSet) ToNodeContainerSelector() map[string]string {
	return withPrefix(subMap(s.labels, "node-container-name", "node-container-pool"))
}

func (s *labelSet) AppName() string {
	return s.getLabel("app-name")
}

func (s *labelSet) AppProcess() string {
	return s.getLabel("app-process")
}

func (s *labelSet) AppPlatform() string {
	return s.getLabel("app-platform")
}

func (s *labelSet) AppReplicas() int {
	replicas, _ := strconv.Atoi(s.getLabel("app-process-replicas"))
	return replicas
}

func (s *labelSet) Restarts() int {
	restarts, _ := strconv.Atoi(s.getLabel("restarts"))
	return restarts
}

func (s *labelSet) BuildImage() string {
	return s.getLabel("build-image")
}

func (s *labelSet) IsStopped() bool {
	stopped, _ := strconv.ParseBool(s.getLabel("is-stopped"))
	return stopped
}

func (s *labelSet) SetRestarts(count int) {
	s.addLabel("restarts", strconv.Itoa(count))
}

func (s *labelSet) SetStopped() {
	s.addLabel("is-stopped", strconv.FormatBool(true))
}

func (s *labelSet) addLabel(k, v string) {
	s.labels[k] = v
}

func (s *labelSet) getLabel(k string) string {
	if v, ok := s.labels[tsuruLabelPrefix+k]; ok {
		return v
	}
	if v, ok := s.labels[k]; ok {
		return v
	}
	if v, ok := s.annotations[tsuruLabelPrefix+k]; ok {
		return v
	}
	if v, ok := s.annotations[k]; ok {
		return v
	}
	return ""
}

func labelSetFromMeta(meta *v1.ObjectMeta) *labelSet {
	return &labelSet{labels: meta.Labels, annotations: meta.Annotations}
}

func podLabels(a provision.App, process, buildImg string, replicas int) (*labelSet, error) {
	routerName, err := a.GetRouterName()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	routerType, _, err := router.Type(routerName)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	set := &labelSet{
		labels: map[string]string{
			"is-tsuru":             strconv.FormatBool(true),
			"is-build":             strconv.FormatBool(buildImg != ""),
			"is-stopped":           strconv.FormatBool(false),
			"app-name":             a.GetName(),
			"app-process":          process,
			"app-process-replicas": strconv.Itoa(replicas),
			"app-platform":         a.GetPlatform(),
			"app-pool":             a.GetPool(),
			"router-name":          routerName,
			"router-type":          routerType,
			"provisioner":          "kubernetes",
		},
		annotations: map[string]string{
			"build-image": buildImg,
		},
	}
	return set, nil
}

func nodeContainerPodLabels(name, pool string) *labelSet {
	return &labelSet{
		labels: map[string]string{
			"is-tsuru":            strconv.FormatBool(true),
			"is-node-container":   strconv.FormatBool(true),
			"provisioner":         "kubernetes",
			"node-container-name": name,
			"node-container-pool": pool,
		},
	}
}
