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

var (
	labelIsTsuru         = "is-tsuru"
	labelIsStopped       = "is-stopped"
	labelIsBuild         = "is-build"
	labelIsDeploy        = "is-deploy"
	labelIsIsolatedRun   = "is-isolated-run"
	labelIsNodeContainer = "is-node-container"
	LabelIsService       = "is-service"

	LabelAppName            = "app-name"
	labelAppProcess         = "app-process"
	labelAppProcessReplicas = "app-process-replicas"
	labelAppPool            = "app-pool"
	labelAppPlatform        = "app-platform"

	labelNodeContainerName = "node-container-name"
	labelNodeContainerPool = "node-container-pool"

	labelNodeInternalPrefix = "tsuru-internal-"
	LabelNodeAddr           = labelNodeInternalPrefix + "node-addr"
	LabelNodePool           = "pool"

	labelRouterName = "router-name"
	labelRouterType = "router-type"

	labelBuildImage = "build-image"
	labelRestarts   = "restarts"

	labelProvisioner = "provisioner"
)

type LabelSet struct {
	Labels map[string]string
	Prefix string
}

func withPrefix(m map[string]string, prefix string) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		if !strings.HasPrefix(k, prefix) {
			k = prefix + k
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
	return withPrefix(s.Labels, s.Prefix)
}

func (s *LabelSet) ToSelector() map[string]string {
	return withPrefix(subMap(s.Labels, LabelAppName, labelAppProcess, labelIsBuild), s.Prefix)
}

func (s *LabelSet) ToAppSelector() map[string]string {
	return withPrefix(subMap(s.Labels, LabelAppName), s.Prefix)
}

func (s *LabelSet) ToNodeContainerSelector() map[string]string {
	return withPrefix(subMap(s.Labels, labelNodeContainerName, labelNodeContainerPool), s.Prefix)
}

func (s *LabelSet) AppName() string {
	return s.getLabel(LabelAppName)
}

func (s *LabelSet) AppProcess() string {
	return s.getLabel(labelAppProcess)
}

func (s *LabelSet) AppPlatform() string {
	return s.getLabel(labelAppPlatform)
}

func (s *LabelSet) NodeAddr() string {
	return s.getLabel(LabelNodeAddr)
}

func (s *LabelSet) NodePool() string {
	return s.getLabel(LabelNodePool)
}

func (s *LabelSet) PublicNodeLabels() map[string]string {
	internalLabels := make(map[string]string)
	for k, v := range s.Labels {
		if strings.HasPrefix(k, labelNodeInternalPrefix) {
			continue
		}
		internalLabels[k] = v
	}
	return internalLabels
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
	return s.getBoolLabel(labelIsStopped)
}

func (s *LabelSet) IsDeploy() bool {
	return s.getBoolLabel(labelIsDeploy)
}

func (s *LabelSet) IsService() bool {
	return s.getBoolLabel(LabelIsService)
}

func (s *LabelSet) IsIsolatedRun() bool {
	return s.getBoolLabel(labelIsIsolatedRun)
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
	if v, ok := s.Labels[s.Prefix+k]; ok {
		return v
	}
	if v, ok := s.Labels[k]; ok {
		return v
	}
	return ""
}

func (s *LabelSet) getBoolLabel(k string) bool {
	v, _ := strconv.ParseBool(s.getLabel(k))
	return v
}

type ServiceLabelsOpts struct {
	App           provision.App
	BuildImage    string
	Process       string
	Provisioner   string
	Replicas      int
	RestartCount  int
	IsDeploy      bool
	IsIsolatedRun bool
	IsBuild       bool
}

func ServiceLabels(opts ServiceLabelsOpts, prefix string) (*LabelSet, error) {
	set, err := ProcessLabels(opts.App, opts.Process, opts.Provisioner, prefix)
	if err != nil {
		return nil, err
	}
	if opts.BuildImage != "" {
		set.Labels[labelBuildImage] = opts.BuildImage
	}
	set.Labels[LabelIsService] = strconv.FormatBool(true)
	set.Labels[labelAppProcessReplicas] = strconv.Itoa(opts.Replicas)
	set.Labels[labelRestarts] = strconv.Itoa(opts.RestartCount)
	set.Labels[labelIsDeploy] = strconv.FormatBool(opts.IsDeploy)
	set.Labels[labelIsIsolatedRun] = strconv.FormatBool(opts.IsIsolatedRun)
	set.Labels[labelIsBuild] = strconv.FormatBool(opts.IsBuild)
	return set, nil
}

func ProcessLabels(a provision.App, process, provisioner, prefix string) (*LabelSet, error) {
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
			LabelAppName:     a.GetName(),
			labelAppProcess:  process,
			labelAppPlatform: a.GetPlatform(),
			labelAppPool:     a.GetPool(),
			labelRouterName:  routerName,
			labelRouterType:  routerType,
			labelProvisioner: provisioner,
		},
		Prefix: prefix,
	}, nil
}

func NodeContainerLabels(name, pool, provisioner, prefix string, extraLabels map[string]string) *LabelSet {
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
	return &LabelSet{Labels: labels, Prefix: prefix}
}
