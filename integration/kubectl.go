// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

// KubectlClusterManager represents a kubectl cluster
// Requires two test environments:
// kubectlctx with the context name
// kubectluser with the cluster user name
type KubectlClusterManager struct {
	env       *Environment
	ipAddress string
}

func (m *KubectlClusterManager) Name() string {
	return "kubectl"
}

func (m *KubectlClusterManager) Provisioner() string {
	return "kubernetes"
}

func (m *KubectlClusterManager) Start() *Result {
	return &Result{}
}

func (m *KubectlClusterManager) Delete() *Result {
	return &Result{}
}

func (m *KubectlClusterManager) certificateFiles() map[string]string {
	kubectl := NewCommand("kubectl", "config", "view", "-o").WithArgs
	res := kubectl("jsonpath='{.users[?(@.name == \"{{.kubectluser}}\")].user.client-certificate}'").Run(m.env)
	clientCert := res.Stdout.String()
	res = kubectl("jsonpath='{.users[?(@.name == \"{{.kubectluser}}\")].user.client-key}'").Run(m.env)
	clientKey := res.Stdout.String()
	res = kubectl("jsonpath='{.clusters[?(@.name == \"{{.kubectlctx}}\")].cluster.certificate-authority}'").Run(m.env)
	caCert := res.Stdout.String()
	return map[string]string{
		"cacert":     caCert,
		"clientcert": clientCert,
		"clientkey":  clientKey,
	}
}

func (m *KubectlClusterManager) UpdateParams() ([]string, bool) {
	kubectl := NewCommand("kubectl").WithArgs
	res := kubectl("config", "view", "-o", "jsonpath='{.clusters[?(@.name == \"{{.kubectlctx}}\")].cluster.server}'").Run(m.env)
	if res.Error != nil || res.ExitCode != 0 {
		return nil, false
	}
	certfiles := m.certificateFiles()
	return []string{"--addr", res.Stdout.String(), "--cacert", certfiles["cacert"], "--clientcert", certfiles["clientcert"], "--clientkey", certfiles["clientkey"]}, false
}
