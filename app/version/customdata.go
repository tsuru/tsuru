// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package version

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	provTypes "github.com/tsuru/tsuru/types/provision"
)

var procfileRegex = regexp.MustCompile(`^([A-Za-z0-9_-]+):\s*(.+)$`)

type customData struct {
	Hooks        *provTypes.TsuruYamlHooks
	Healthcheck  *provTypes.TsuruYamlHealthcheck
	Startupcheck *provTypes.TsuruYamlStartupcheck
	Kubernetes   *tsuruYamlKubernetesConfig
	Processes    []provTypes.TsuruYamlProcess
}

type tsuruYamlKubernetesConfig struct {
	Groups []tsuruYamlKubernetesGroup
}

type tsuruYamlKubernetesGroup struct {
	Name      string
	Processes []tsuruYamlKubernetesProcess
}

type tsuruYamlKubernetesProcess struct {
	Name  string
	Ports []tsuruYamlKubernetesProcessPortConfig
}

type tsuruYamlKubernetesProcessPortConfig struct {
	Name       string `json:"name,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
	Port       int    `json:"port,omitempty"`
	TargetPort int    `json:"target_port,omitempty" bson:"target_port,omitempty"`
}

func unmarshalYamlData(data map[string]interface{}) (provTypes.TsuruYamlData, error) {
	if data == nil {
		return provTypes.TsuruYamlData{}, nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return provTypes.TsuruYamlData{}, err
	}
	custom := customData{}
	err = json.Unmarshal(b, &custom)
	if err != nil {
		return provTypes.TsuruYamlData{}, err
	}

	result := provTypes.TsuruYamlData{
		Hooks:        custom.Hooks,
		Processes:    custom.Processes,
		Healthcheck:  custom.Healthcheck,
		Startupcheck: custom.Startupcheck,
	}
	if custom.Kubernetes == nil {
		return result, nil
	}

	result.Kubernetes = &provTypes.TsuruYamlKubernetesConfig{}
	for _, g := range custom.Kubernetes.Groups {
		group := provTypes.TsuruYamlKubernetesGroup{}
		for _, proc := range g.Processes {
			group[proc.Name] = provTypes.TsuruYamlKubernetesProcessConfig{
				Ports: make([]provTypes.TsuruYamlKubernetesProcessPortConfig, len(proc.Ports)),
			}
			for i, port := range proc.Ports {
				group[proc.Name].Ports[i] = provTypes.TsuruYamlKubernetesProcessPortConfig(port)
			}
		}
		if result.Kubernetes.Groups == nil {
			result.Kubernetes.Groups = map[string]provTypes.TsuruYamlKubernetesGroup{
				g.Name: group,
			}
		} else {
			result.Kubernetes.Groups[g.Name] = group
		}
	}
	return result, nil
}

func marshalCustomData(data map[string]interface{}) (map[string]interface{}, error) {
	if len(data) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var yamlData provTypes.TsuruYamlData
	err = json.Unmarshal(b, &yamlData)
	if err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	for k, v := range data {
		if v != nil {
			result[k] = v
		}
	}
	result["hooks"] = yamlData.Hooks
	result["healthcheck"] = yamlData.Healthcheck
	result["startupcheck"] = yamlData.Startupcheck
	if len(yamlData.Processes) > 0 {
		result["processes"] = yamlData.Processes
	}
	if yamlData.Kubernetes == nil {
		return result, nil
	}
	kubeConfig := &tsuruYamlKubernetesConfig{}

	for groupName, groupData := range yamlData.Kubernetes.Groups {
		group := tsuruYamlKubernetesGroup{Name: groupName}
		for procName, procData := range groupData {
			proc := tsuruYamlKubernetesProcess{Name: procName}
			for _, port := range procData.Ports {
				proc.Ports = append(proc.Ports, tsuruYamlKubernetesProcessPortConfig(port))
			}
			group.Processes = append(group.Processes, proc)
		}
		if kubeConfig.Groups == nil {
			kubeConfig.Groups = []tsuruYamlKubernetesGroup{group}
		} else {
			kubeConfig.Groups = append(kubeConfig.Groups, group)
		}
	}
	result["kubernetes"] = kubeConfig
	return result, nil
}

func processesFromCustomData(customData map[string]interface{}) (map[string][]string, error) {
	var processes map[string][]string
	if data, ok := customData["processes"]; ok {
		procs := data.(map[string]interface{})
		processes = make(map[string][]string, len(procs))
		for name, command := range procs {
			switch cmdType := command.(type) {
			case string:
				processes[name] = []string{cmdType}
			case []string:
				processes[name] = cmdType
			case []interface{}:
				for _, v := range cmdType {
					if vStr, ok := v.(string); ok {
						processes[name] = append(processes[name], vStr)
					}
				}
			default:
				return nil, fmt.Errorf("invalid type for process entry: %T", cmdType)
			}
		}
		delete(customData, "processes")
		delete(customData, "procfile")
	}
	if data, ok := customData["procfile"]; ok {
		processes = GetProcessesFromProcfile(data.(string))
		if len(processes) == 0 {
			return nil, errors.New("invalid Procfile")
		}
		delete(customData, "procfile")
	}
	return processes, nil
}

func GetProcessesFromProcfile(strProcfile string) map[string][]string {
	procfile := strings.Split(strProcfile, "\n")
	processes := make(map[string][]string, len(procfile))
	for _, process := range procfile {
		if p := procfileRegex.FindStringSubmatch(process); p != nil {
			if p[1] == "" || p[2] == "" {
				continue
			}
			processes[p[1]] = []string{strings.TrimSpace(p[2])}
		}
	}
	return processes
}

func GetProcessesFromYamlProcess(yamlProcesses []provTypes.TsuruYamlProcess) map[string][]string {
	processes := make(map[string][]string, len(yamlProcesses))
	for _, process := range yamlProcesses {
		if process.Command == "" {
			continue
		}
		processes[process.Name] = []string{strings.TrimSpace(process.Command)}
	}
	return processes
}
