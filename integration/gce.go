// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"time"
)

var clusterName = "integration-test"
var projectID = os.Getenv("GCE_PROJECT_ID")

type gceClusterManager struct {
	ipAddress string
}

func (g *gceClusterManager) Name() string {
	return "gce"
}

func (g *gceClusterManager) Provisioner() string {
	return "kubernetes"
}

func (g *gceClusterManager) IP(env *Environment) string {
	return g.ipAddress
}

func (g *gceClusterManager) Start(env *Environment) *Result {
	gcloud := NewCommand("gcloud").WithArgs
	res := gcloud("container", "clusters", "create", clusterName, "--project", projectID, "--num-nodes", "1").WithTimeout(15 * time.Minute).Run(env)
	regex := regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`)
	parts := regex.FindStringSubmatch(res.Stdout.String())
	if len(parts) == 1 {
		g.ipAddress = parts[0]
	}
	return res
}

func (g *gceClusterManager) Delete(env *Environment) *Result {
	gcloud := NewCommand("gcloud").WithArgs
	return gcloud("container", "clusters", "delete", clusterName, "--project", projectID, "--async").WithInput("y").Run(env)
}

func (g *gceClusterManager) credentials(env *Environment) (map[string]string, error) {
	credentials := make(map[string]string)
	gcloud := NewCommand("gcloud").WithArgs
	res := gcloud("container", "clusters", "describe", clusterName, "--project", projectID).Run(env)
	regex := regexp.MustCompile(`username: (.*)`)
	parts := regex.FindStringSubmatch(res.Stdout.String())
	if len(parts) == 1 {
		credentials["username"] = parts[0]
	}
	regex = regexp.MustCompile(`password: (.*)`)
	parts = regex.FindStringSubmatch(res.Stdout.String())
	if len(parts) == 1 {
		credentials["password"] = parts[0]
	}
	regex = regexp.MustCompile(`clusterCaCertificate`)
	parts = regex.FindStringSubmatch(res.Stdout.String())
	if len(parts) == 1 {
		contents, err := base64.StdEncoding.DecodeString(parts[0])
		if err != nil {
			return credentials, err
		}
		tmpfile, err := ioutil.TempFile("", "gce-ca")
		if err != nil {
			return credentials, err
		}
		if _, err := tmpfile.Write(contents); err != nil {
			return credentials, err
		}
		if err := tmpfile.Close(); err != nil {
			return credentials, err
		}
		credentials["certificateFilename"] = tmpfile.Name()
	}
	return credentials, nil
}

func (g *gceClusterManager) UpdateParams(env *Environment) []string {
	address := fmt.Sprintf("https://%s", g.IP(env))
	credentials, err := g.credentials(env)
	if err != nil {
		return []string{}
	}
	return []string{"--addr", address, "--custom", "username=" + credentials["username"], "--custom", "password=" + credentials["password"], "--cacert", credentials["certificateFilename"]}
}
