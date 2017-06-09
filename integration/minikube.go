// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"
	"os"
	"regexp"
	"time"
)

type minikubeClusterManager struct {
	ipAddress string
}

func (m *minikubeClusterManager) Name() string {
	return "minikube"
}

func (m *minikubeClusterManager) Provisioner() string {
	return "kubernetes"
}

func (m *minikubeClusterManager) IP(env *Environment) string {
	if len(m.ipAddress) == 0 {
		minikube := NewCommand("minikube").WithArgs
		res := minikube("ip").Run(env)
		if res.Error != nil || res.ExitCode != 0 {
			return ""
		}
		regex := regexp.MustCompile(`([\d.]+)`)
		parts := regex.FindStringSubmatch(res.Stdout.String())
		if len(parts) != 2 {
			return ""
		}
		m.ipAddress = parts[1]
	}
	return m.ipAddress
}

func (m *minikubeClusterManager) Start(env *Environment) *Result {
	minikube := NewCommand("minikube").WithArgs
	return minikube("start", `--insecure-registry="192.168.0.0/16"`).WithTimeout(15 * time.Minute).Run(env)
}

func (m *minikubeClusterManager) Delete(env *Environment) *Result {
	minikube := NewCommand("minikube").WithArgs
	return minikube("delete").WithTimeout(5 * time.Minute).Run(env)
}

func (m *minikubeClusterManager) certificateFiles() map[string]string {
	minikubeDir := fmt.Sprintf("%s/.minikube", os.Getenv("HOME"))
	return map[string]string{
		"cacert":     minikubeDir + "/ca.crt",
		"clientcert": minikubeDir + "/apiserver.crt",
		"clientkey":  minikubeDir + "/apiserver.key",
	}
}

func (m *minikubeClusterManager) UpdateParams(env *Environment) []string {
	address := fmt.Sprintf("https://%s:8443", m.IP(env))
	certfiles := m.certificateFiles()
	return []string{"--addr", address, "--cacert", certfiles["cacert"], "--clientcert", certfiles["clientcert"], "--clientkey", certfiles["clientkey"]}
}
