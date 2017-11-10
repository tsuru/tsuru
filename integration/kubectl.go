// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import "fmt"

// KubectlClusterManager represents a kubectl context
type KubectlClusterManager struct {
	env     *Environment
	context string
	config  string
	binary  string
}

func (m *KubectlClusterManager) Name() string {
	return "kubectl"
}

func (m *KubectlClusterManager) Provisioner() string {
	return kubernetesProvisioner
}

func (m *KubectlClusterManager) Start() *Result {
	return &Result{}
}

func (m *KubectlClusterManager) Delete() *Result {
	return &Result{}
}

func (m *KubectlClusterManager) certificateFiles() map[string]string {
	return map[string]string{
		"cacert":     m.getConfig(fmt.Sprintf("{.clusters[?(@.name == \"%s\")].cluster.certificate-authority}", m.getUser())),
		"clientcert": m.getConfig(fmt.Sprintf("{.users[?(@.name == \"%s\")].user.client-certificate}", m.getUser())),
		"clientkey":  m.getConfig(fmt.Sprintf("{.users[?(@.name == \"%s\")].user.client-key}", m.getCluster())),
	}
}

func (m *KubectlClusterManager) UpdateParams() ([]string, bool) {
	cluster := m.getCluster()
	addr := m.getConfig(fmt.Sprintf("{.clusters[?(@.name == \"%s\")].cluster.server}", cluster))
	certfiles := m.certificateFiles()
	return []string{"--addr", addr, "--cacert", certfiles["cacert"], "--clientcert", certfiles["clientcert"], "--clientkey", certfiles["clientkey"]}, false
}

func (m *KubectlClusterManager) getBinary() string {
	if m.binary == "" {
		return "kubectl"
	}
	return m.binary
}

func (m *KubectlClusterManager) getUser() string {
	return m.getConfig(fmt.Sprintf("{.contexts[?(@.name == \"%s\")].context.user}", m.context))
}

func (m *KubectlClusterManager) getCluster() string {
	return m.getConfig(fmt.Sprintf("{.contexts[?(@.name == \"%s\")].context.cluster}", m.context))
}

func (m *KubectlClusterManager) getConfig(jsonpath string) string {
	kubectl := NewCommand(m.getBinary(), "--kubeconfig", m.config, "config", "view", "-o").WithArgs
	res := kubectl(fmt.Sprintf("jsonpath='%s'", jsonpath)).Run(m.env)
	if res.Error != nil || res.ExitCode != 0 {
		if m.env.VerboseLevel() > 1 {
			fmt.Printf("failed to get config %s: %v (exit %d)\n", res.Command.Args, res.Error, res.ExitCode)
		}
		return ""
	}
	return res.Stdout.String()
}
