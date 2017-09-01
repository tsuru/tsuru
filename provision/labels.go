// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/set"
)

var (
	labelIsTsuru         = "is-tsuru"
	labelIsStopped       = "is-stopped"
	labelIsAsleep        = "is-asleep"
	labelIsBuild         = "is-build"
	labelIsDeploy        = "is-deploy"
	labelIsIsolatedRun   = "is-isolated-run"
	labelIsNodeContainer = "is-node-container"
	labelIsService       = "is-service"

	labelAppName            = "app-name"
	labelAppProcess         = "app-process"
	labelAppProcessReplicas = "app-process-replicas"
	labelAppPool            = "app-pool"
	labelAppPlatform        = "app-platform"

	labelNodeContainerName = "node-container-name"
	labelNodeContainerPool = "node-container-pool"

	labelNodeInternalPrefix = "tsuru-internal-"
	labelNodeAddr           = labelNodeInternalPrefix + "node-addr"
	LabelNodePool           = PoolMetadataName

	labelVolumeName = "volume-name"
	labelVolumePool = "volume-pool"
	labelVolumePlan = "volume-plan"

	labelRouterName = "router-name"
	labelRouterType = "router-type"

	labelBuildImage = "build-image"
	labelRestarts   = "restarts"

	labelProvisioner = "provisioner"

	labelBuilder = "builder"

	LabelClusterMetadata = "tsuru.io/cluster"
)

type LabelSet struct {
	Labels map[string]string
	Prefix string
}

func (s *LabelSet) ToLabels() map[string]string {
	return withPrefix(s.Labels, s.Prefix)
}

func (s *LabelSet) ToSelector() map[string]string {
	return withPrefix(subMap(s.Labels, labelAppName, labelAppProcess, labelIsBuild, labelIsIsolatedRun), s.Prefix)
}

func (s *LabelSet) ToAppSelector() map[string]string {
	return withPrefix(subMap(s.Labels, labelAppName), s.Prefix)
}

func (s *LabelSet) ToNodeContainerSelector() map[string]string {
	return withPrefix(subMap(s.Labels, labelNodeContainerName, labelNodeContainerPool), s.Prefix)
}

func (s *LabelSet) ToNodeSelector() map[string]string {
	return withPrefix(subMap(s.Labels, LabelNodePool, labelNodeAddr), s.Prefix)
}

func (s *LabelSet) ToNodeByPoolSelector() map[string]string {
	return withPrefix(subMap(s.Labels, LabelNodePool), s.Prefix)
}

func (s *LabelSet) ToIsServiceSelector() map[string]string {
	return withPrefix(subMap(s.Labels, labelIsService), s.Prefix)
}

func (s *LabelSet) ToVolumeSelector() map[string]string {
	return withPrefix(subMap(s.Labels, labelVolumeName), s.Prefix)
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

func (s *LabelSet) NodeAddr() string {
	return s.getLabel(labelNodeAddr)
}

func (s *LabelSet) NodePool() string {
	return s.getLabel(LabelNodePool)
}

func (s *LabelSet) PublicNodeLabels() map[string]string {
	internalLabels := make(map[string]string)
	for k, v := range s.Labels {
		if strings.HasPrefix(k, labelNodeInternalPrefix) ||
			strings.HasPrefix(k, s.Prefix+labelNodeInternalPrefix) {
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

func (s *LabelSet) IsAsleep() bool {
	return s.getBoolLabel(labelIsAsleep)
}

func (s *LabelSet) IsDeploy() bool {
	return s.getBoolLabel(labelIsDeploy)
}

func (s *LabelSet) IsService() bool {
	return s.getBoolLabel(labelIsService)
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

func (s *LabelSet) SetAsleep() {
	s.addLabel(labelIsAsleep, strconv.FormatBool(true))
}

func (s *LabelSet) SetIsService() {
	s.addLabel(labelIsService, strconv.FormatBool(true))
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
	App      App
	Process  string
	Replicas int
	ServiceLabelExtendedOpts
}

type ServiceLabelExtendedOpts struct {
	Provisioner   string
	Prefix        string
	BuildImage    string
	IsDeploy      bool
	IsIsolatedRun bool
	IsBuild       bool
	Builder       string
}

func ExtendServiceLabels(set *LabelSet, opts ServiceLabelExtendedOpts) {
	set.Prefix = opts.Prefix
	if opts.BuildImage != "" {
		set.Labels[labelBuildImage] = opts.BuildImage
	}
	set.Labels[labelProvisioner] = opts.Provisioner
	set.Labels[labelIsService] = strconv.FormatBool(true)
	set.Labels[labelIsDeploy] = strconv.FormatBool(opts.IsDeploy)
	set.Labels[labelIsIsolatedRun] = strconv.FormatBool(opts.IsIsolatedRun)
	set.Labels[labelIsBuild] = strconv.FormatBool(opts.IsBuild)
	set.Labels[labelBuilder] = opts.Builder
}

func ServiceLabels(opts ServiceLabelsOpts) (*LabelSet, error) {
	set, err := ProcessLabels(ProcessLabelsOpts{
		App:     opts.App,
		Process: opts.Process,
	})
	if err != nil {
		return nil, err
	}
	set.Labels[labelAppProcessReplicas] = strconv.Itoa(opts.Replicas)
	ExtendServiceLabels(set, opts.ServiceLabelExtendedOpts)
	return set, nil
}

type ProcessLabelsOpts struct {
	App         App
	Process     string
	Provisioner string
	Builder     string
	Prefix      string
}

func ProcessLabels(opts ProcessLabelsOpts) (*LabelSet, error) {
	routerName, err := opts.App.GetRouterName()
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
			labelAppName:     opts.App.GetName(),
			labelAppProcess:  opts.Process,
			labelAppPlatform: opts.App.GetPlatform(),
			labelAppPool:     opts.App.GetPool(),
			labelRouterName:  routerName,
			labelRouterType:  routerType,
			labelProvisioner: opts.Provisioner,
			labelBuilder:     opts.Builder,
		},
		Prefix: opts.Prefix,
	}, nil
}

type NodeContainerLabelsOpts struct {
	Name         string
	CustomLabels map[string]string
	Pool         string
	Provisioner  string
	Prefix       string
}

func NodeContainerLabels(opts NodeContainerLabelsOpts) *LabelSet {
	labels := map[string]string{
		labelIsTsuru:           strconv.FormatBool(true),
		labelIsNodeContainer:   strconv.FormatBool(true),
		labelProvisioner:       opts.Provisioner,
		labelNodeContainerName: opts.Name,
		labelNodeContainerPool: opts.Pool,
	}
	for k, v := range opts.CustomLabels {
		labels[k] = v
	}
	return &LabelSet{Labels: labels, Prefix: opts.Prefix}
}

type NodeLabelsOpts struct {
	Addr         string
	Pool         string
	Prefix       string
	CustomLabels map[string]string
}

func NodeLabels(opts NodeLabelsOpts) *LabelSet {
	labels := map[string]string{}
	for k, v := range opts.CustomLabels {
		labels[k] = v
	}
	for _, r := range []string{LabelNodePool, labelNodeAddr} {
		delete(labels, r)
		delete(labels, opts.Prefix+r)
	}
	labels[LabelNodePool] = opts.Pool
	if opts.Addr != "" {
		labels[labelNodeAddr] = opts.Addr
	}
	return &LabelSet{Labels: labels, Prefix: opts.Prefix}
}

type VolumeLabelsOpts struct {
	Name        string
	Provisioner string
	Pool        string
	Plan        string
	Prefix      string
}

func VolumeLabels(opts VolumeLabelsOpts) *LabelSet {
	labels := map[string]string{
		labelIsTsuru:     strconv.FormatBool(true),
		labelProvisioner: opts.Provisioner,
		labelVolumeName:  opts.Name,
		labelVolumePool:  opts.Pool,
		labelVolumePlan:  opts.Plan,
	}
	return &LabelSet{Labels: labels, Prefix: opts.Prefix}
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
