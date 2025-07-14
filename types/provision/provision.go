// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

import (
	"errors"
	"maps"
	"sort"
	"strings"

	"github.com/tsuru/tsuru/types/router"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const defaultHealthcheckScheme = "http"

var ErrProcessNotFound = errors.New("process name could not be found on YAML data")

type TsuruYamlData struct {
	Hooks        *TsuruYamlHooks            `json:"hooks,omitempty" bson:",omitempty"`
	Healthcheck  *TsuruYamlHealthcheck      `json:"healthcheck,omitempty" bson:",omitempty"`
	Startupcheck *TsuruYamlStartupcheck     `json:"startupcheck,omitempty" bson:",omitempty"`
	Kubernetes   *TsuruYamlKubernetesConfig `json:"kubernetes,omitempty" bson:",omitempty"`
	Processes    []TsuruYamlProcess         `json:"processes,omitempty" bson:",omitempty"`
}

type TsuruYamlHooks struct {
	Restart TsuruYamlRestartHooks `json:"restart" bson:",omitempty"`
	Build   []string              `json:"build" bson:",omitempty"`
}

type TsuruYamlRestartHooks struct {
	Before []string `json:"before" bson:",omitempty"`
	After  []string `json:"after" bson:",omitempty"`
}

type TsuruYamlHealthcheck struct {
	Headers              map[string]string `json:"headers,omitempty" bson:",omitempty"`
	Path                 string            `json:"path"`
	Scheme               string            `json:"scheme"`
	Command              []string          `json:"command,omitempty" bson:",omitempty"`
	AllowedFailures      int               `json:"allowed_failures,omitempty" yaml:"allowed_failures" bson:"allowed_failures,omitempty"`
	IntervalSeconds      int               `json:"interval_seconds,omitempty" yaml:"interval_seconds" bson:"interval_seconds,omitempty"`
	TimeoutSeconds       int               `json:"timeout_seconds,omitempty" yaml:"timeout_seconds" bson:"timeout_seconds,omitempty"`
	DeployTimeoutSeconds int               `json:"deploy_timeout_seconds,omitempty" yaml:"deploy_timeout_seconds" bson:"deploy_timeout_seconds,omitempty"`
	ForceRestart         bool              `json:"force_restart,omitempty" yaml:"force_restart" bson:"force_restart,omitempty"`
}

type TsuruYamlStartupcheck struct {
	Headers              map[string]string `json:"headers,omitempty" bson:",omitempty"`
	Path                 string            `json:"path"`
	Scheme               string            `json:"scheme"`
	Command              []string          `json:"command,omitempty" bson:",omitempty"`
	AllowedFailures      int               `json:"allowed_failures,omitempty" yaml:"allowed_failures" bson:"allowed_failures,omitempty"`
	IntervalSeconds      int               `json:"interval_seconds,omitempty" yaml:"interval_seconds" bson:"interval_seconds,omitempty"`
	TimeoutSeconds       int               `json:"timeout_seconds,omitempty" yaml:"timeout_seconds" bson:"timeout_seconds,omitempty"`
	DeployTimeoutSeconds int               `json:"deploy_timeout_seconds,omitempty" yaml:"deploy_timeout_seconds" bson:"deploy_timeout_seconds,omitempty"`
}

type TsuruYamlProcess struct {
	Healthcheck  *TsuruYamlHealthcheck  `json:"healthcheck,omitempty" bson:",omitempty"`
	Startupcheck *TsuruYamlStartupcheck `json:"startupcheck,omitempty" bson:",omitempty"`
	Name         string                 `json:"name"`
	Command      string                 `json:"command" yaml:"command" bson:"command"`
}

type TsuruYamlKubernetesConfig struct {
	Groups map[string]TsuruYamlKubernetesGroup `json:"groups,omitempty"`
}

func (in *TsuruYamlKubernetesConfig) DeepCopyInto(out *TsuruYamlKubernetesConfig) {
	if in.Groups == nil {
		return
	}
	if out.Groups == nil {
		out.Groups = make(map[string]TsuruYamlKubernetesGroup)
	}
	maps.Copy(out.Groups, in.Groups)
}

func (in *TsuruYamlKubernetesConfig) DeepCopy() *TsuruYamlKubernetesConfig {
	out := &TsuruYamlKubernetesConfig{}
	in.DeepCopyInto(out)
	return out
}

type TsuruYamlKubernetesGroup map[string]TsuruYamlKubernetesProcessConfig

type TsuruYamlKubernetesProcessConfig struct {
	Ports []TsuruYamlKubernetesProcessPortConfig `json:"ports"`
}

type TsuruYamlKubernetesProcessPortConfig struct {
	Name       string `json:"name,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
	Port       int    `json:"port,omitempty"`
	TargetPort int    `json:"target_port,omitempty"`
}

func (y TsuruYamlData) ToRouterHC() router.HealthcheckData {
	hc := y.Healthcheck
	if hc == nil {
		return router.HealthcheckData{
			Path: "/",
		}
	}
	return router.HealthcheckData{
		Path: hc.Path,
	}
}

func (y TsuruYamlData) GetCheckConfigsFromProcessName(process string) (*TsuruYamlHealthcheck, *TsuruYamlStartupcheck, error) {
	for _, tsuruProcessData := range y.Processes {
		if tsuruProcessData.Name == process {
			return tsuruProcessData.Healthcheck, tsuruProcessData.Startupcheck, nil
		}
	}
	return nil, nil, ErrProcessNotFound
}

func (y *TsuruYamlKubernetesConfig) GetProcessConfigs(procName string) *TsuruYamlKubernetesProcessConfig {
	for _, group := range y.Groups {
		for p, proc := range group {
			if p == procName {
				return &proc
			}
		}
	}
	return nil
}

func (y *TsuruYamlHealthcheck) EnsureDefaults() error {
	if y.Scheme == "" {
		y.Scheme = defaultHealthcheckScheme
	}
	if y.IntervalSeconds == 0 {
		y.IntervalSeconds = 10
	}
	if y.TimeoutSeconds == 0 {
		y.TimeoutSeconds = 60
	}
	if y.AllowedFailures == 0 {
		y.AllowedFailures = 3
	}

	return nil
}

func (y *TsuruYamlHealthcheck) IsEmpty() bool {
	return y.Path == "" && len(y.Command) == 0
}

func (y *TsuruYamlHealthcheck) AssembleProbe(port int) (*apiv1.Probe, error) {
	if err := y.EnsureDefaults(); err != nil {
		return nil, err
	}
	headers := []apiv1.HTTPHeader{}
	for header, value := range y.getHeaders() {
		headers = append(headers, apiv1.HTTPHeader{Name: header, Value: value})
	}
	sort.Slice(headers, func(i, j int) bool { return headers[i].Name < headers[j].Name })
	formatedScheme := strings.ToUpper(y.Scheme)
	probe := &apiv1.Probe{
		FailureThreshold: y.getAllowedFailures(),
		PeriodSeconds:    y.getIntervalSeconds(),
		TimeoutSeconds:   y.getTimeoutSeconds(),
		ProbeHandler:     apiv1.ProbeHandler{},
	}
	if y.Path != "" {
		path := y.Path
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		probe.ProbeHandler.HTTPGet = &apiv1.HTTPGetAction{
			Path:        path,
			Port:        intstr.FromInt(port),
			Scheme:      apiv1.URIScheme(formatedScheme),
			HTTPHeaders: headers,
		}
	} else {
		probe.ProbeHandler.Exec = &apiv1.ExecAction{
			Command: y.Command,
		}
	}
	return probe, nil
}

func (y *TsuruYamlHealthcheck) getHeaders() map[string]string {
	result := make(map[string]string)
	maps.Copy(result, y.Headers)
	return result
}

func (y *TsuruYamlHealthcheck) getAllowedFailures() int32 {
	return int32(y.AllowedFailures)
}

func (y *TsuruYamlHealthcheck) getIntervalSeconds() int32 {
	return int32(y.IntervalSeconds)
}

func (y *TsuruYamlHealthcheck) getTimeoutSeconds() int32 {
	return int32(y.TimeoutSeconds)
}

func (y *TsuruYamlStartupcheck) EnsureDefaults() error {
	if y.Scheme == "" {
		y.Scheme = defaultHealthcheckScheme
	}
	if y.IntervalSeconds == 0 {
		y.IntervalSeconds = 10
	}
	if y.TimeoutSeconds == 0 {
		y.TimeoutSeconds = 60
	}
	if y.AllowedFailures == 0 {
		y.AllowedFailures = 3
	}

	return nil
}

func (y *TsuruYamlStartupcheck) IsEmpty() bool {
	return y.Path == "" && len(y.Command) == 0
}

func (y *TsuruYamlStartupcheck) AssembleProbe(port int) (*apiv1.Probe, error) {
	if err := y.EnsureDefaults(); err != nil {
		return nil, err
	}
	headers := []apiv1.HTTPHeader{}
	for header, value := range y.getHeaders() {
		headers = append(headers, apiv1.HTTPHeader{Name: header, Value: value})
	}
	sort.Slice(headers, func(i, j int) bool { return headers[i].Name < headers[j].Name })
	formatedScheme := strings.ToUpper(y.Scheme)
	probe := &apiv1.Probe{
		FailureThreshold: y.getAllowedFailures(),
		PeriodSeconds:    y.getIntervalSeconds(),
		TimeoutSeconds:   y.getTimeoutSeconds(),
		ProbeHandler:     apiv1.ProbeHandler{},
	}
	if y.Path != "" {
		path := y.Path
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		probe.ProbeHandler.HTTPGet = &apiv1.HTTPGetAction{
			Path:        path,
			Port:        intstr.FromInt(port),
			Scheme:      apiv1.URIScheme(formatedScheme),
			HTTPHeaders: headers,
		}
	} else {
		probe.ProbeHandler.Exec = &apiv1.ExecAction{
			Command: y.Command,
		}
	}
	return probe, nil
}

func (y *TsuruYamlStartupcheck) getHeaders() map[string]string {
	result := make(map[string]string)
	maps.Copy(result, y.Headers)
	return result
}

func (y *TsuruYamlStartupcheck) getAllowedFailures() int32 {
	return int32(y.AllowedFailures)
}

func (y *TsuruYamlStartupcheck) getIntervalSeconds() int32 {
	return int32(y.IntervalSeconds)
}

func (y *TsuruYamlStartupcheck) getTimeoutSeconds() int32 {
	return int32(y.TimeoutSeconds)
}
