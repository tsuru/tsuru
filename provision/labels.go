// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/router"
	"github.com/tsuru/tsuru/set"
)

var (
	labelIsTsuru           = "is-tsuru"
	labelIsStopped         = "is-stopped"
	labelIsAsleep          = "is-asleep"
	labelIsBuild           = "is-build"
	labelIsDeploy          = "is-deploy"
	labelIsIsolatedRun     = "is-isolated-run"
	labelIsIsolatedRunNew  = "is-isolated-run-version"
	labelIsNodeContainer   = "is-node-container"
	labelIsService         = "is-service"
	labelIsHeadlessService = "is-headless-service"
	labelIsRoutable        = "is-routable"

	labelAppName            = "app-name"
	labelAppProcess         = "app-process"
	labelAppProcessReplicas = "app-process-replicas"
	LabelAppPool            = "app-pool"
	labelAppPlatform        = "app-platform"
	labelAppVersion         = "app-version"

	labelNodeContainerName = "node-container-name"
	labelNodeContainerPool = "node-container-pool"

	labelNodeInternalPrefix = "internal-"
	labelNodeAddr           = labelNodeInternalPrefix + "node-addr"
	LabelNodePool           = PoolMetadataName
	labelNodeIaaSID         = IaaSIDMetadataName

	labelVolumeName = "volume-name"
	labelVolumePool = "volume-pool"
	labelVolumePlan = "volume-plan"
	labelVolumeTeam = "volume-team"

	labelRouterName = "router-name"
	labelRouterType = "router-type"

	labelBuildImage = "build-image"
	labelRestarts   = "restarts"

	labelProvisioner = "provisioner"

	labelBuilder = "builder"

	labelClusterMetadata = "tsuru.io/cluster"
)

type LabelSet struct {
	Labels    map[string]string
	RawLabels map[string]string
	Prefix    string
}

func (s *LabelSet) DeepCopy() *LabelSet {
	newLabels := &LabelSet{Prefix: s.Prefix}
	newLabels.Labels = make(map[string]string)
	newLabels.RawLabels = make(map[string]string)
	for k, v := range s.Labels {
		newLabels.Labels[k] = v
	}
	for k, v := range s.RawLabels {
		newLabels.RawLabels[k] = v
	}
	return newLabels
}

func (s *LabelSet) ToLabels() map[string]string {
	result := withPrefix(s.Labels, s.Prefix)
	for k, v := range s.RawLabels {
		result[k] = v
	}
	return result
}

func (s *LabelSet) ToVersionSelector() map[string]string {
	return withPrefix(subMap(s.Labels, labelAppName, labelAppProcess, labelIsBuild, labelIsIsolatedRun, labelIsIsolatedRunNew, labelAppVersion), s.Prefix)
}

func (s *LabelSet) ToBaseSelector() map[string]string {
	return withPrefix(subMap(s.Labels, labelAppName, labelAppProcess, labelIsBuild, labelIsIsolatedRun), s.Prefix)
}

func (s *LabelSet) ToAllVersionsSelector() map[string]string {
	return withPrefix(subMap(s.Labels, labelAppName, labelAppProcess, labelIsBuild), s.Prefix)
}

func (s *LabelSet) ToRoutableSelector() map[string]string {
	return withPrefix(subMap(s.Labels, labelAppName, labelAppProcess, labelIsBuild, labelIsRoutable), s.Prefix)
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

func (s *LabelSet) AppPool() string {
	return s.getLabel(LabelAppPool)
}

func (s *LabelSet) NodeAddr() string {
	return s.getLabel(labelNodeAddr)
}

func (s *LabelSet) NodePool() string {
	return s.getLabel(LabelNodePool)
}

func (s *LabelSet) NodeIaaSID() string {
	return s.getLabel(labelNodeIaaSID)
}

func (s *LabelSet) AppVersion() int {
	v, _ := strconv.Atoi(s.getLabel(labelAppVersion))
	return v
}

func (s *LabelSet) WithoutVersion() *LabelSet {
	ns := s.without(labelAppVersion)
	delete(ns.RawLabels, "version")
	return ns
}

func (s *LabelSet) WithoutAppReplicas() *LabelSet {
	return s.without(labelAppProcessReplicas)
}

func (s *LabelSet) WithoutRoutable() *LabelSet {
	return s.without(labelIsRoutable)
}

func (s *LabelSet) WithoutIsolated() *LabelSet {
	return s.without(labelIsIsolatedRun).without(labelIsIsolatedRunNew)
}

func (s *LabelSet) without(key string) *LabelSet {
	ns := s.DeepCopy()
	delete(ns.Labels, key)
	delete(ns.Labels, s.Prefix+key)
	return ns
}

func (s *LabelSet) ReplaceIsIsolatedRunWithNew() {
	if !s.hasLabel(labelIsIsolatedRunNew) {
		s.Labels[labelIsIsolatedRunNew] = s.Labels[labelIsIsolatedRun]
	}
	delete(s.Labels, labelIsIsolatedRun)
}

func (s *LabelSet) ReplaceIsIsolatedNewRunWithBase() {
	if !s.hasLabel(labelIsIsolatedRun) {
		s.Labels[labelIsIsolatedRun] = s.Labels[labelIsIsolatedRunNew]
	}
	delete(s.Labels, labelIsIsolatedRunNew)
}

func filterByPrefix(labels, raw map[string]string, prefix string, withPrefix bool) map[string]string {
	result := make(map[string]string, len(labels))
	for k, v := range labels {
		hasPrefix := strings.HasPrefix(k, prefix)
		if hasPrefix == withPrefix {
			result[k] = v
		}
	}
	if !withPrefix {
		for k, v := range raw {
			result[k] = v
		}
	}
	return result
}

func (s *LabelSet) NodeMetadata() map[string]string {
	m := filterByPrefix(s.Labels, s.RawLabels, s.Prefix, true)
	for k := range m {
		if strings.HasPrefix(k, s.Prefix+labelNodeInternalPrefix) {
			delete(m, k)
		}
	}
	return m
}

func (s *LabelSet) NodeMetadataNoPrefix() map[string]string {
	m := filterByPrefix(s.Labels, s.RawLabels, s.Prefix, true)
	for k, v := range m {
		delete(m, k)
		if !strings.HasPrefix(k, s.Prefix+labelNodeInternalPrefix) {
			m[strings.TrimPrefix(k, s.Prefix)] = v
		}

	}
	return m
}

func (s *LabelSet) NodeExtraData(cluster string) map[string]string {
	m := filterByPrefix(s.Labels, s.RawLabels, s.Prefix, false)
	for k, v := range s.Labels {
		if strings.HasPrefix(k, s.Prefix+labelNodeInternalPrefix) {
			m[k] = v
		}
	}
	if cluster != "" {
		m[labelClusterMetadata] = cluster
	}
	return m
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
	return s.getBoolLabel(labelIsIsolatedRun) || s.getBoolLabel(labelIsIsolatedRunNew)
}

func (s *LabelSet) IsBase() bool {
	return s.hasLabel(labelIsIsolatedRun)
}

func (s *LabelSet) IsRoutable() bool {
	return s.getBoolLabel(labelIsRoutable)
}

func (s *LabelSet) IsHeadlessService() bool {
	return s.getBoolLabel(labelIsHeadlessService)
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

func (s *LabelSet) SetIsHeadlessService() {
	s.addLabel(labelIsHeadlessService, strconv.FormatBool(true))
}

func (s *LabelSet) SetIsRoutable() {
	s.addLabel(labelIsRoutable, strconv.FormatBool(true))
}

func (s *LabelSet) ToggleIsRoutable(isRoutable bool) {
	s.addLabel(labelIsRoutable, strconv.FormatBool(isRoutable))
}

func (s *LabelSet) SetBuildImage(image string) {
	s.addLabel(labelBuildImage, image)
}

func (s *LabelSet) SetVersion(version int) {
	s.addLabel(labelAppVersion, strconv.Itoa(version))
	if s.RawLabels == nil {
		s.RawLabels = make(map[string]string)
	}
	s.RawLabels["version"] = fmt.Sprintf("v%d", version)
}

func (s *LabelSet) addLabel(k, v string) {
	if s.Labels == nil {
		s.Labels = make(map[string]string)
	}
	s.Labels[k] = v
}

func (s *LabelSet) hasLabel(k string) bool {
	if _, ok := s.Labels[s.Prefix+k]; ok {
		return ok
	}
	if _, ok := s.Labels[k]; ok {
		return ok
	}
	return false
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
	Version  int
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
		App:      opts.App,
		Process:  opts.Process,
		IsDeploy: opts.IsDeploy,
	})
	if err != nil {
		return nil, err
	}
	set.Labels[labelAppProcessReplicas] = strconv.Itoa(opts.Replicas)
	if set.RawLabels == nil {
		set.RawLabels = make(map[string]string)
	}
	if opts.Version != 0 {
		set.SetVersion(opts.Version)
	}
	ExtendServiceLabels(set, opts.ServiceLabelExtendedOpts)
	return set, nil
}

func SplitServiceLabelsAnnotations(ls *LabelSet) (labels *LabelSet, ann *LabelSet) {
	labels = &LabelSet{Prefix: ls.Prefix, Labels: map[string]string{}, RawLabels: map[string]string{}}
	ann = &LabelSet{Prefix: ls.Prefix, Labels: map[string]string{}}
	annKeys := map[string]struct{}{
		labelRouterName: {},
		labelRouterType: {},
	}
	for k, v := range ls.RawLabels {
		labels.RawLabels[k] = v
	}
	for k, v := range ls.Labels {
		if _, ok := annKeys[k]; ok {
			ann.Labels[k] = v
		} else {
			labels.Labels[k] = v
		}
	}
	return labels, ann
}

type ProcessLabelsOpts struct {
	App         App
	Process     string
	Provisioner string
	Builder     string
	Prefix      string
	IsDeploy    bool
}

func ProcessLabels(opts ProcessLabelsOpts) (*LabelSet, error) {
	var routerNames, routerTypes []string
	for _, appRouter := range opts.App.GetRouters() {
		routerType, _, err := router.Type(appRouter.Name)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		routerNames = append(routerNames, appRouter.Name)
		routerTypes = append(routerTypes, routerType)
	}
	return &LabelSet{
		Labels: map[string]string{
			labelIsTsuru:     strconv.FormatBool(true),
			labelIsStopped:   strconv.FormatBool(false),
			labelIsDeploy:    strconv.FormatBool(opts.IsDeploy),
			labelAppName:     opts.App.GetName(),
			labelAppProcess:  opts.Process,
			labelAppPlatform: opts.App.GetPlatform(),
			LabelAppPool:     opts.App.GetPool(),
			labelRouterName:  strings.Join(routerNames, ","),
			labelRouterType:  strings.Join(routerTypes, ","),
			labelProvisioner: opts.Provisioner,
			labelBuilder:     opts.Builder,
		},
		Prefix: opts.Prefix,
	}, nil
}

type ServiceAccountLabelsOpts struct {
	App               App
	NodeContainerName string
	Provisioner       string
	Prefix            string
}

func ServiceAccountLabels(opts ServiceAccountLabelsOpts) *LabelSet {
	labelMap := map[string]string{
		labelIsTsuru:     strconv.FormatBool(true),
		labelProvisioner: opts.Provisioner,
	}
	if opts.App == nil {
		labelMap[labelNodeContainerName] = opts.NodeContainerName
	} else {
		labelMap[labelAppName] = opts.App.GetName()
	}
	return &LabelSet{
		Labels: labelMap,
		Prefix: opts.Prefix,
	}
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
	IaaSID       string
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
	for _, r := range []string{LabelNodePool, labelNodeAddr, labelNodeIaaSID} {
		delete(labels, r)
		delete(labels, opts.Prefix+r)
	}
	labels[LabelNodePool] = opts.Pool
	if opts.Addr != "" {
		labels[labelNodeAddr] = opts.Addr
	}
	if opts.IaaSID != "" {
		labels[labelNodeIaaSID] = opts.IaaSID
	}
	return &LabelSet{Labels: labels, Prefix: opts.Prefix}
}

type VolumeLabelsOpts struct {
	Name        string
	Provisioner string
	Pool        string
	Plan        string
	Team        string
	Prefix      string
}

func VolumeLabels(opts VolumeLabelsOpts) *LabelSet {
	labels := map[string]string{
		labelIsTsuru:     strconv.FormatBool(true),
		labelProvisioner: opts.Provisioner,
		labelVolumeName:  opts.Name,
		labelVolumePool:  opts.Pool,
		labelVolumePlan:  opts.Plan,
		labelVolumeTeam:  opts.Team,
	}
	return &LabelSet{Labels: labels, Prefix: opts.Prefix}
}

type ImageBuildLabelsOpts struct {
	Name         string
	CustomLabels map[string]string
	Provisioner  string
	Prefix       string
	IsBuild      bool
}

func ImageBuildLabels(opts ImageBuildLabelsOpts) *LabelSet {
	labels := map[string]string{
		labelIsTsuru:     strconv.FormatBool(true),
		labelProvisioner: opts.Provisioner,
		labelIsBuild:     strconv.FormatBool(opts.IsBuild),
	}
	for k, v := range opts.CustomLabels {
		labels[k] = v
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

func IsServiceLabelSet(prefix string) *LabelSet {
	labels := map[string]string{
		labelIsTsuru:   strconv.FormatBool(true),
		labelIsService: strconv.FormatBool(true),
	}
	return &LabelSet{Labels: labels, Prefix: prefix}
}
