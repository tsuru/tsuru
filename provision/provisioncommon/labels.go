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

var (
	labelIsTsuru         = "is-tsuru"
	labelIsStopped       = "is-stopped"
	labelIsBuild         = "is-build"
	labelIsDeploy        = "is-deploy"
	labelIsIsolatedRun   = "is-isolated-run"
	labelIsNodeContainer = "is-node-container"

	labelAppName            = "app-name"
	labelAppProcess         = "app-process"
	labelAppProcessReplicas = "app-process-replicas"
	labelAppPool            = "app-pool"
	labelAppPlatform        = "app-platform"

	labelNodeContainerName = "node-container-name"
	labelNodeContainerPool = "node-container-pool"

	labelRouterName = "router-name"
	labelRouterType = "router-type"

	labelBuildImage = "build-image"
	labelRestarts   = "restarts"

	labelProvisioner = "provisioner"
)

type LabelSet struct {
	Labels map[string]string
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

func (s *LabelSet) ToSelector() map[string]string {
	return withPrefix(subMap(s.Labels, labelAppName, labelAppProcess, labelIsBuild))
}

func (s *LabelSet) ToAppSelector() map[string]string {
	return withPrefix(subMap(s.Labels, labelAppName))
}

func (s *LabelSet) ToNodeContainerSelector() map[string]string {
	return withPrefix(subMap(s.Labels, labelNodeContainerName, labelNodeContainerPool))
}

func (s *LabelSet) AppName() string {
	return s.getLabel(labelAppName)
}

func (s *LabelSet) AppProcess() string {
	return s.getLabel(labelAppProcess)
}

func (s *LabelSet) AppPlatform() string {
	return s.getLabel(labelAppPlatform)
}

func (s *LabelSet) AppReplicas() int {
	replicas, _ := strconv.Atoi(s.getLabel(labelAppProcessReplicas))
	return replicas
}

func (s *LabelSet) Restarts() int {
	restarts, _ := strconv.Atoi(s.getLabel(labelRestarts))
	return restarts
}

func (s *LabelSet) BuildImage() string {
	return s.getLabel(labelBuildImage)
}

func (s *LabelSet) IsStopped() bool {
	stopped, _ := strconv.ParseBool(s.getLabel(labelIsStopped))
	return stopped
}

func (s *LabelSet) SetRestarts(count int) {
	s.addLabel(labelRestarts, strconv.Itoa(count))
}

func (s *LabelSet) SetStopped() {
	s.addLabel(labelIsStopped, strconv.FormatBool(true))
}

func (s *LabelSet) SetBuildImage(image string) {
	s.addLabel(labelBuildImage, image)
}

func (s *LabelSet) addLabel(k, v string) {
	if s.Labels == nil {
		s.Labels = make(map[string]string)
	}
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
		labelBuildImage: buildImg,
	}
	set.Labels[labelIsBuild] = strconv.FormatBool(buildImg != "")
	set.Labels[labelAppProcessReplicas] = strconv.Itoa(replicas)
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
			labelIsTsuru:     strconv.FormatBool(true),
			labelIsStopped:   strconv.FormatBool(false),
			labelAppName:     a.GetName(),
			labelAppProcess:  process,
			labelAppPlatform: a.GetPlatform(),
			labelAppPool:     a.GetPool(),
			labelRouterName:  routerName,
			labelRouterType:  routerType,
			labelProvisioner: provisioner,
		},
	}, nil
}

func NodeContainerLabels(name, pool, provisioner string, extraLabels map[string]string) *LabelSet {
	labels := map[string]string{
		labelIsTsuru:           strconv.FormatBool(true),
		labelIsNodeContainer:   strconv.FormatBool(true),
		labelProvisioner:       provisioner,
		labelNodeContainerName: name,
		labelNodeContainerPool: pool,
	}
	for k, v := range extraLabels {
		labels[k] = v
	}
	return &LabelSet{Labels: labels}
}
