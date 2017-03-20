// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provisioncommon

import (
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/set"
)

const (
	tsuruLabelPrefix = "tsuru.io/"
)

type LabelSet struct {
	Labels      map[string]string
	Annotations map[string]string
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

func (s *LabelSet) ToLabels() map[string]string {
	return withPrefix(s.Labels)
}

func (s *LabelSet) ToAnnotations() map[string]string {
	return withPrefix(s.Annotations)
}

func (s *LabelSet) ToSelector() map[string]string {
	return withPrefix(subMap(s.Labels, "app-name", "app-process", "is-build"))
}

func (s *LabelSet) ToAppSelector() map[string]string {
	return withPrefix(subMap(s.Labels, "app-name"))
}

func (s *LabelSet) ToNodeContainerSelector() map[string]string {
	return withPrefix(subMap(s.Labels, "node-container-name", "node-container-pool"))
}

func (s *LabelSet) AppName() string {
	return s.getLabel("app-name")
}

func (s *LabelSet) AppProcess() string {
	return s.getLabel("app-process")
}

func (s *LabelSet) AppPlatform() string {
	return s.getLabel("app-platform")
}

func (s *LabelSet) AppReplicas() int {
	replicas, _ := strconv.Atoi(s.getLabel("app-process-replicas"))
	return replicas
}

func (s *LabelSet) Restarts() int {
	restarts, _ := strconv.Atoi(s.getLabel("restarts"))
	return restarts
}

func (s *LabelSet) BuildImage() string {
	return s.getLabel("build-image")
}

func (s *LabelSet) IsStopped() bool {
	stopped, _ := strconv.ParseBool(s.getLabel("is-stopped"))
	return stopped
}

func (s *LabelSet) SetRestarts(count int) {
	s.addLabel("restarts", strconv.Itoa(count))
}

func (s *LabelSet) SetStopped() {
	s.addLabel("is-stopped", strconv.FormatBool(true))
}

func (s *LabelSet) addLabel(k, v string) {
	s.Labels[k] = v
}

func (s *LabelSet) getLabel(k string) string {
	if v, ok := s.Labels[tsuruLabelPrefix+k]; ok {
		return v
	}
	if v, ok := s.Labels[k]; ok {
		return v
	}
	if v, ok := s.Annotations[tsuruLabelPrefix+k]; ok {
		return v
	}
	if v, ok := s.Annotations[k]; ok {
		return v
	}
	return ""
}

func PodLabels(a provision.App, process, buildImg string, replicas int) (*LabelSet, error) {
	set, err := ProcessLabels(a, process, "kubernetes")
	if err != nil {
		return nil, err
	}
	set.Annotations = map[string]string{
		"build-image": buildImg,
	}
	set.Labels["is-build"] = strconv.FormatBool(buildImg != "")
	set.Labels["app-process-replicas"] = strconv.Itoa(replicas)
	return set, nil
}

func ProcessLabels(a provision.App, process, provisioner string) (*LabelSet, error) {
	routerName, err := a.GetRouterName()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	routerType, _, err := router.Type(routerName)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &LabelSet{
		Labels: map[string]string{
			"is-tsuru":     strconv.FormatBool(true),
			"is-stopped":   strconv.FormatBool(false),
			"app-name":     a.GetName(),
			"app-process":  process,
			"app-platform": a.GetPlatform(),
			"app-pool":     a.GetPool(),
			"router-name":  routerName,
			"router-type":  routerType,
			"provisioner":  provisioner,
		},
	}, nil
}

func NodeContainerLabels(name, pool, provisioner string, extraLabels map[string]string) *LabelSet {
	labels := map[string]string{
		"is-tsuru":            strconv.FormatBool(true),
		"is-node-container":   strconv.FormatBool(true),
		"provisioner":         provisioner,
		"node-container-name": name,
		"node-container-pool": pool,
	}
	for k, v := range extraLabels {
		labels[k] = v
	}
	return &LabelSet{Labels: labels}
}
