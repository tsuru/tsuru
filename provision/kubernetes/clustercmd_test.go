// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ajg/form"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/cmdtest"
	"gopkg.in/check.v1"
)

func (s *S) TestKubernetesClusterUpdateRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"c1"},
	}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			err := req.ParseForm()
			c.Assert(err, check.IsNil)
			dec := form.NewDecoder(nil)
			dec.IgnoreCase(true)
			dec.IgnoreUnknownKeys(true)
			var cluster Cluster
			err = dec.DecodeValues(&cluster, req.Form)
			c.Assert(err, check.IsNil)
			c.Assert(cluster, check.DeepEquals, Cluster{
				Name:              "c1",
				CaCert:            []byte("cadata"),
				ClientCert:        []byte("certdata"),
				ClientKey:         []byte("keydata"),
				ExplicitNamespace: "tsuru",
				Addresses:         []string{"addr1", "addr2"},
				Pools:             []string{"p1", "p2"},
				Default:           true,
			})
			return req.URL.Path == "/1.3/kubernetes/clusters" && req.Method == "POST"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	myCmd := kubernetesClusterUpdate{}
	dir, err := ioutil.TempDir("", "tsuru")
	c.Assert(err, check.IsNil)
	defer os.RemoveAll(dir)
	err = ioutil.WriteFile(filepath.Join(dir, "ca"), []byte("cadata"), 0600)
	c.Assert(err, check.IsNil)
	err = ioutil.WriteFile(filepath.Join(dir, "cert"), []byte("certdata"), 0600)
	c.Assert(err, check.IsNil)
	err = ioutil.WriteFile(filepath.Join(dir, "key"), []byte("keydata"), 0600)
	c.Assert(err, check.IsNil)
	err = myCmd.Flags().Parse(true, []string{
		"--cacert", filepath.Join(dir, "ca"),
		"--clientcert", filepath.Join(dir, "cert"),
		"--clientkey", filepath.Join(dir, "key"),
		"--namespace", "tsuru",
		"--addr", "addr1",
		"--addr", "addr2",
		"--pool", "p1",
		"--pool", "p2",
		"--default",
	})
	c.Assert(err, check.IsNil)
	err = myCmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "Cluster successfully updated.\n")
}

func (s *S) TestKubernetesClusterListRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	clusters := []Cluster{{
		Name:              "c1",
		Addresses:         []string{"addr1", "addr2"},
		CaCert:            []byte("cacert"),
		ClientCert:        []byte("clientcert"),
		ClientKey:         []byte("clientkey"),
		ExplicitNamespace: "ns1",
		Default:           true,
	}, {
		Name:      "c2",
		Addresses: []string{"addr3"},
		Default:   false,
		Pools:     []string{"p1", "p2"},
	}}
	data, err := json.Marshal(clusters)
	c.Assert(err, check.IsNil)
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Message: string(data), Status: http.StatusOK},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/1.3/kubernetes/clusters" && req.Method == "GET"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	myCmd := kubernetesClusterList{}
	err = myCmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, `+------+-----------+-----------+---------+-------+
| Name | Addresses | Namespace | Default | Pools |
+------+-----------+-----------+---------+-------+
| c1   | addr1     | ns1       | true    |       |
|      | addr2     |           |         |       |
+------+-----------+-----------+---------+-------+
| c2   | addr3     | default   | false   | p1    |
|      |           |           |         | p2    |
+------+-----------+-----------+---------+-------+
`)
}

func (s *S) TestKubernetesClusterRemoveRun(c *check.C) {
	var stdout, stderr bytes.Buffer
	context := cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
		Args:   []string{"c1"},
	}
	trans := &cmdtest.ConditionalTransport{
		Transport: cmdtest.Transport{Status: http.StatusNoContent},
		CondFunc: func(req *http.Request) bool {
			return req.URL.Path == "/1.3/kubernetes/clusters/c1" && req.Method == "DELETE"
		},
	}
	manager := cmd.NewManager("admin", "0.1", "admin-ver", &stdout, &stderr, nil, nil)
	client := cmd.NewClient(&http.Client{Transport: trans}, nil, manager)
	myCmd := kubernetesClusterRemove{}
	err := myCmd.Run(&context, client)
	c.Assert(err, check.IsNil)
	c.Assert(stdout.String(), check.Equals, "Cluster successfully removed.\n")
}
