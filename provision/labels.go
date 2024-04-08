// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/tsuru/tsuru/set"
	jobTypes "github.com/tsuru/tsuru/types/job"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	LabelIsBuild = "is-build"

	labelIsTsuru           = "is-tsuru"
	labelIsStopped         = "is-stopped"
	labelIsIsolatedRun     = "is-isolated-run"
	labelIsIsolatedRunNew  = "is-isolated-run-version"
	labelIsNodeContainer   = "is-node-container"
	labelIsService         = "is-service"
	labelIsHeadlessService = "is-headless-service"
	labelIsRoutable        = "is-routable"

	LabelAppName      = "app-name"
	LabelAppProcess   = "app-process"
	LabelAppPool      = "app-pool"
	LabelAppPlatform  = "app-platform"
	LabelAppVersion   = "app-version"
	LabelAppTeamOwner = "app-team"

	LabelIsJob        = "is-job"
	LabelJobName      = "job-name"
	LabelJobPool      = "job-pool"
	LabelJobTeamOwner = "job-team"
	LabelJobIsManual  = "job-manual"

	labelNodeContainerName = "node-container-name"
	labelNodeContainerPool = "node-container-pool"

	labelNodeInternalPrefix = "internal-"
	labelNodeAddr           = labelNodeInternalPrefix + "node-addr"
	LabelNodePool           = PoolMetadataName

	labelVolumeName = "volume-name"
	labelVolumePool = "volume-pool"
	labelVolumePlan = "volume-plan"
	labelVolumeTeam = "volume-team"

	labelRestarts = "restarts"

	labelClusterMetadata = "tsuru.io/cluster"

	labelCustomTagsPrefix = "custom-tag-"

	tsuruLabelPrefix = "tsuru.io/"
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

func (s *LabelSet) Merge(override *LabelSet) *LabelSet {
	if s == nil {
		return nil
	}
	l := s.DeepCopy()
	if override == nil {
		return l
	}
	for k, v := range override.Labels {
		l.Labels[k] = v
	}
	for k, v := range override.RawLabels {
		l.RawLabels[k] = v
	}
	if override.Prefix != "" {
		l.Prefix = override.Prefix
	}
	return l
}

func (s *LabelSet) ToLabels() map[string]string {
	result := withPrefix(s.Labels, s.Prefix)
	for k, v := range s.RawLabels {
		result[k] = v
	}
	return result
}

func (s *LabelSet) ToVersionSelector() map[string]string {
	return withPrefix(subMap(s.Labels, LabelAppName, LabelAppProcess, LabelIsBuild, labelIsIsolatedRun, labelIsIsolatedRunNew, LabelAppVersion), s.Prefix)
}

func (s *LabelSet) ToBaseSelector() map[string]string {
	return withPrefix(subMap(s.Labels, LabelAppName, LabelAppProcess, LabelIsBuild, labelIsIsolatedRun), s.Prefix)
}

func (s *LabelSet) ToAllVersionsSelector() map[string]string {
	return withPrefix(subMap(s.Labels, LabelAppName, LabelAppProcess, LabelIsBuild), s.Prefix)
}

func (s *LabelSet) ToRoutableSelector() map[string]string {
	return withPrefix(subMap(s.Labels, LabelAppName, LabelAppProcess, LabelIsBuild, labelIsRoutable), s.Prefix)
}

func (s *LabelSet) LeanPodLabels() map[string]string {
	result := withPrefix(subMap(s.Labels, LabelAppName, LabelAppProcess, labelIsRoutable, labelIsTsuru, LabelIsBuild, labelIsIsolatedRun, LabelAppVersion, LabelAppTeamOwner), s.Prefix)

	for k, v := range s.RawLabels {
		result[k] = v
	}
	return result
}

func (s *LabelSet) ToAppSelector() map[string]string {
	return withPrefix(subMap(s.Labels, LabelAppName), s.Prefix)
}

func (s *LabelSet) ToJobSelector() map[string]string {
	return withPrefix(subMap(s.Labels, LabelJobName), s.Prefix)
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

func (s *LabelSet) ToHPASelector() map[string]string {
	keys := []string{labelIsTsuru, LabelAppName}
	if s.getLabel(LabelAppProcess) != "" {
		keys = append(keys, LabelAppProcess)
	}
	return withPrefix(subMap(s.Labels, keys...), s.Prefix)
}

func (s *LabelSet) ToPDBSelector() map[string]string {
	return s.ToHPASelector()
}

func (s *LabelSet) AppName() string {
	return s.getLabel(LabelAppName)
}

func (s *LabelSet) AppProcess() string {
	return s.getLabel(LabelAppProcess)
}

func (s *LabelSet) AppPlatform() string {
	return s.getLabel(LabelAppPlatform)
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

func (s *LabelSet) AppVersion() int {
	v, _ := strconv.Atoi(s.getLabel(LabelAppVersion))
	return v
}

func (s *LabelSet) WithoutVersion() *LabelSet {
	ns := s.without(LabelAppVersion)
	delete(ns.RawLabels, "version")
	delete(ns.RawLabels, "app.kubernetes.io/version")
	return ns
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

func (s *LabelSet) JobName() string {
	return s.getLabel(LabelJobName)
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

func (s *LabelSet) Restarts() int {
	restarts, _ := strconv.Atoi(s.getLabel(labelRestarts))
	return restarts
}

func (s *LabelSet) IsStopped() bool {
	return s.getBoolLabel(labelIsStopped)
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

func (s *LabelSet) SetVersion(version int) {
	s.addLabel(LabelAppVersion, strconv.Itoa(version))
	if s.RawLabels == nil {
		s.RawLabels = make(map[string]string)
	}
	versionStr := fmt.Sprintf("v%d", version)
	s.RawLabels["version"] = versionStr
	s.RawLabels["app.kubernetes.io/version"] = versionStr
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
	App     App
	Process string
	Version int
	ServiceLabelExtendedOpts
}

type ServiceLabelExtendedOpts struct {
	Prefix        string
	IsIsolatedRun bool
	IsBuild       bool
}

func ExtendServiceLabels(set *LabelSet, opts ServiceLabelExtendedOpts) {
	set.Prefix = opts.Prefix
	set.Labels[labelIsService] = strconv.FormatBool(true)
	set.Labels[labelIsIsolatedRun] = strconv.FormatBool(opts.IsIsolatedRun)
	set.Labels[LabelIsBuild] = strconv.FormatBool(opts.IsBuild)
}

func ServiceLabels(ctx context.Context, opts ServiceLabelsOpts) (*LabelSet, error) {
	set, err := ProcessLabels(ctx, ProcessLabelsOpts{
		App:     opts.App,
		Process: opts.Process,
	})
	if err != nil {
		return nil, err
	}
	if set.RawLabels == nil {
		set.RawLabels = map[string]string{
			"app.kubernetes.io/name":       opts.App.GetName(),
			"app.kubernetes.io/component":  "tsuru-app",
			"app.kubernetes.io/managed-by": "tsuru",
		}
		if opts.Process != "" {
			appProcessName := AppProcessName(opts.App, opts.Process, 0, "")
			set.RawLabels["app"] = appProcessName
			set.RawLabels["app.kubernetes.io/instance"] = appProcessName
		}
	}
	if opts.Version != 0 {
		set.SetVersion(opts.Version)
	}
	ExtendServiceLabels(set, opts.ServiceLabelExtendedOpts)
	return set, nil
}

func JobLabels(ctx context.Context, job *jobTypes.Job) *LabelSet {
	return &LabelSet{
		Labels: map[string]string{
			labelIsTsuru:      strconv.FormatBool(true),
			LabelJobName:      job.Name,
			LabelJobTeamOwner: job.TeamOwner,
			LabelJobPool:      job.Pool,
			LabelIsJob:        strconv.FormatBool(true),
			LabelJobIsManual:  strconv.FormatBool(job.Spec.Manual),
			labelIsService:    strconv.FormatBool(true),
			LabelIsBuild:      strconv.FormatBool(false),
		},
		RawLabels: map[string]string{
			// Reference about these labels: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
			"app.kubernetes.io/name":       "tsuru-job",
			"app.kubernetes.io/instance":   job.Name,
			"app.kubernetes.io/component":  "job",
			"app.kubernetes.io/managed-by": "tsuru",
		},
		Prefix: tsuruLabelPrefix,
	}
}

type ProcessLabelsOpts struct {
	App     App
	Process string
	Prefix  string
}

func ProcessLabels(ctx context.Context, opts ProcessLabelsOpts) (*LabelSet, error) {
	ls := &LabelSet{
		Labels: map[string]string{
			labelIsTsuru:      strconv.FormatBool(true),
			labelIsStopped:    strconv.FormatBool(false),
			LabelAppName:      opts.App.GetName(),
			LabelAppTeamOwner: opts.App.GetTeamOwner(),
			LabelAppProcess:   opts.Process,
			LabelAppPlatform:  opts.App.GetPlatform(),
			LabelAppPool:      opts.App.GetPool(),
		},
		Prefix: opts.Prefix,
	}
	for _, tag := range opts.App.ListTags() {
		parts := strings.SplitN(tag, "=", 2)
		var key, value string
		if len(parts) > 0 {
			key = parts[0]
		}
		if len(parts) > 1 {
			value = parts[1]
		}
		if key == "" {
			continue
		}
		key = labelCustomTagsPrefix + key
		if len(validation.IsQualifiedName(key)) > 0 {
			// Ignoring tags that are not valid identifiers for labels or annotations
			continue
		}
		ls.Labels[key] = value
	}
	return ls, nil
}

type ServiceAccountLabelsOpts struct {
	App               App
	Job               *jobTypes.Job
	NodeContainerName string
	Prefix            string
}

func ServiceAccountLabels(opts ServiceAccountLabelsOpts) *LabelSet {
	labelMap := map[string]string{
		labelIsTsuru: strconv.FormatBool(true),
	}
	if opts.App != nil {
		labelMap[LabelAppName] = opts.App.GetName()
	} else {
		labelMap[LabelJobName] = opts.Job.Name
	}
	return &LabelSet{
		Labels: labelMap,
		Prefix: opts.Prefix,
	}
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
	Name   string
	Pool   string
	Plan   string
	Team   string
	Prefix string
}

func VolumeLabels(opts VolumeLabelsOpts) *LabelSet {
	labels := map[string]string{
		labelIsTsuru:    strconv.FormatBool(true),
		labelVolumeName: opts.Name,
		labelVolumePool: opts.Pool,
		labelVolumePlan: opts.Plan,
		labelVolumeTeam: opts.Team,
	}
	return &LabelSet{Labels: labels, Prefix: opts.Prefix}
}

type ImageBuildLabelsOpts struct {
	Name         string
	CustomLabels map[string]string
	Prefix       string
	IsBuild      bool
}

func ImageBuildLabels(opts ImageBuildLabelsOpts) *LabelSet {
	labels := map[string]string{
		labelIsTsuru: strconv.FormatBool(true),
		LabelIsBuild: strconv.FormatBool(opts.IsBuild),
	}
	for k, v := range opts.CustomLabels {
		labels[k] = v
	}
	return &LabelSet{Labels: labels, Prefix: opts.Prefix}
}

type PDBLabelsOpts struct {
	App     App
	Prefix  string
	Process string
}

func PDBLabels(opts PDBLabelsOpts) *LabelSet {
	return &LabelSet{
		Labels: map[string]string{
			labelIsTsuru:      strconv.FormatBool(true),
			LabelAppName:      opts.App.GetName(),
			LabelAppProcess:   opts.Process,
			LabelAppTeamOwner: opts.App.GetTeamOwner(),
		},
		Prefix: opts.Prefix,
	}
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

func ServiceLabelSet(prefix string) *LabelSet {
	labels := map[string]string{
		labelIsTsuru:   strconv.FormatBool(true),
		labelIsService: strconv.FormatBool(true),
	}
	return &LabelSet{Labels: labels, Prefix: prefix}
}

func TsuruJobLabelSet(prefix string) *LabelSet {
	labels := map[string]string{
		labelIsTsuru: strconv.FormatBool(true),
		LabelIsJob:   strconv.FormatBool(true),
	}
	return &LabelSet{Labels: labels, Prefix: prefix}
}

var kubeNameRegex = regexp.MustCompile(`(?i)[^a-z0-9.-]`)

func ValidKubeName(name string) string {
	return strings.ToLower(kubeNameRegex.ReplaceAllString(name, "-"))
}

func AppProcessName(a App, process string, version int, suffix string) string {
	const kubeLabelNameMaxLen = 55

	name := ValidKubeName(a.GetName())
	processVersion := ValidKubeName(process)
	if version > 0 {
		processVersion = fmt.Sprintf("%s-v%d", processVersion, version)
	} else if suffix != "" {
		processVersion = fmt.Sprintf("%s-%s", processVersion, suffix)
	}
	label := fmt.Sprintf("%s-%s", name, processVersion)
	if len(label) > kubeLabelNameMaxLen {
		h := sha256.New()
		h.Write([]byte(processVersion))
		hash := fmt.Sprintf("%x", h.Sum(nil))
		maxLen := kubeLabelNameMaxLen - len(name) - 1
		if len(hash) > maxLen {
			hash = hash[:maxLen]
		}
		label = fmt.Sprintf("%s-%s", name, hash)
	}
	return label
}
