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

// MinikubeClusterManager represents a minikube local instance
type MinikubeClusterManager struct {
	env       *Environment
	ipAddress string
}

func (m *MinikubeClusterManager) Name() string {
	return "minikube"
}

func (m *MinikubeClusterManager) Provisioner() string {
	return kubernetesProvisioner
}

func (m *MinikubeClusterManager) IP() string {
	if len(m.ipAddress) == 0 {
		minikube := NewCommand("minikube").WithArgs
		res := minikube("ip").Run(m.env)
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

func (m *MinikubeClusterManager) Start() *Result {
	minikube := NewCommand("minikube").WithArgs
	return minikube("start", `--insecure-registry="192.168.0.0/16"`).WithTimeout(15 * time.Minute).Run(m.env)
}

func (m *MinikubeClusterManager) Delete() *Result {
	minikube := NewCommand("minikube").WithArgs
	return minikube("delete").WithTimeout(5 * time.Minute).Run(m.env)
}

func (m *MinikubeClusterManager) certificateFiles() map[string]string {
	minikubeDir := fmt.Sprintf("%s/.minikube", os.Getenv("HOME"))
	return map[string]string{
		"cacert":     minikubeDir + "/ca.crt",
		"clientcert": minikubeDir + "/apiserver.crt",
		"clientkey":  minikubeDir + "/apiserver.key",
	}
}

func (m *MinikubeClusterManager) UpdateParams() ([]string, bool) {
	address := fmt.Sprintf("https://%s:8443", m.IP())
	certfiles := m.certificateFiles()
	return []string{"--addr", address, "--cacert", certfiles["cacert"], "--clientcert", certfiles["clientcert"], "--clientkey", certfiles["clientkey"]}, false
}
