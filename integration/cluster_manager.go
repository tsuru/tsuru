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

type clusterManager interface {
	Provisioner() string
	IP(env *Environment) string
	Address(env *Environment) string
	Start(env *Environment) *Result
	Delete(env *Environment) *Result
	CertificateFiles() map[string]string
}

type kubernetesClusterManager struct {
	ipAddress string
}

func (k *kubernetesClusterManager) Provisioner() string {
	return "kubernetes"
}

func (k *kubernetesClusterManager) IP(env *Environment) string {
	if len(k.ipAddress) == 0 {
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
		k.ipAddress = parts[1]
	}
	return k.ipAddress
}

func (k *kubernetesClusterManager) Address(env *Environment) string {
	return fmt.Sprintf("https://%s:8443", k.IP(env))
}

func (k *kubernetesClusterManager) Start(env *Environment) *Result {
	minikube := NewCommand("minikube").WithArgs
	return minikube("start", `--insecure-registry="192.168.0.0/16"`).WithTimeout(15 * time.Minute).Run(env)
}

func (k *kubernetesClusterManager) Delete(env *Environment) *Result {
	minikube := NewCommand("minikube").WithArgs
	return minikube("delete").WithTimeout(5 * time.Minute).Run(env)
}

func (k *kubernetesClusterManager) CertificateFiles() map[string]string {
	minikubeDir := fmt.Sprintf("%s/.minikube", os.Getenv("HOME"))
	return map[string]string{
		"cacert":     minikubeDir + "/ca.crt",
		"clientcert": minikubeDir + "/apiserver.crt",
		"clientkey":  minikubeDir + "/apiserver.key",
	}
}
