// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mesos

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/andygrunwald/megos"
	"github.com/gambol99/go-marathon"
	"github.com/pkg/errors"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision/cluster"
)

const (
	tokenClusterKey    = "token"
	userClusterKey     = "username"
	passwordClusterKey = "password"
	mesosPortKey       = "mesosport"
	marathonPortKey    = "marathonport"

	mesosDefaultPort    = "5050"
	marathonDefaultPort = "8080"
)

var (
	// hookMarathonClient allow overriding client for testing mocks
	hookMarathonClient = func(cli marathon.Marathon) marathon.Marathon {
		return cli
	}

	// hookMesosClient allow overriding client for testing mocks
	hookMesosClient = func(cli mesosClient) mesosClient {
		return cli
	}
)

type mesosClient interface {
	GetSlavesFromCluster() (*megos.State, error)
}

type clusterClient struct {
	*cluster.Cluster
	mesos    mesosClient
	marathon marathon.Marathon
}

type authRoundTripper struct {
	http.RoundTripper
	token string
}

func (a *authRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("Authorization", "token="+a.token)
	return a.RoundTripper.RoundTrip(r)
}

func newClusterClient(c *cluster.Cluster) (*clusterClient, error) {
	config := marathon.NewDefaultConfig()
	if len(c.Addresses) == 0 {
		return nil, errors.New("no addresses for cluster")
	}
	urls := make([]*url.URL, len(c.Addresses))
	var err error
	for i, urlRaw := range c.Addresses {
		urls[i], err = url.Parse(urlRaw)
		if err != nil {
			return nil, err
		}
	}
	baseUrl := urls[rand.Intn(len(urls))]
	config.HTTPClient = tsuruNet.Dial5Full300ClientNoKeepAlive
	mesosHttpCli := *config.HTTPClient
	if c.CustomData != nil {
		config.HTTPBasicAuthUser = c.CustomData[userClusterKey]
		config.HTTPBasicPassword = c.CustomData[passwordClusterKey]
		config.DCOSToken = c.CustomData[tokenClusterKey]
	}
	if config.DCOSToken != "" {
		config.URL = fmt.Sprintf("%s/marathon", strings.TrimSuffix(baseUrl.String(), "/"))
		for _, u := range urls {
			u.Path = "/mesos"
		}
		mesosHttpCli.Transport = &authRoundTripper{
			RoundTripper: mesosHttpCli.Transport,
			token:        config.DCOSToken,
		}
	} else {
		var mesosPort, marathonPort string
		if c.CustomData != nil {
			mesosPort = c.CustomData[mesosPortKey]
			marathonPort = c.CustomData[marathonPortKey]
		}
		if mesosPort == "" {
			mesosPort = mesosDefaultPort
		}
		if marathonPort == "" {
			marathonPort = marathonDefaultPort
		}
		config.URL = fmt.Sprintf("%s://%s:%s", baseUrl.Scheme, tsuruNet.URLToHost(baseUrl.String()), marathonPort)
		for _, u := range urls {
			host, _, _ := net.SplitHostPort(u.Host)
			if host == "" {
				host = u.Host
			}
			u.Host = fmt.Sprintf("%s:%s", host, mesosPort)
		}
	}
	marathonClient, err := marathon.NewClient(config)
	if err != nil {
		return nil, err
	}
	mesosClient := megos.NewClient(urls, &mesosHttpCli)
	return &clusterClient{
		Cluster:  c,
		marathon: hookMarathonClient(marathonClient),
		mesos:    hookMesosClient(mesosClient),
	}, nil
}

func allClusters() ([]*clusterClient, error) {
	clusters, err := cluster.ForProvisioner(provisionerName)
	if err != nil {
		return nil, err
	}
	clients := make([]*clusterClient, len(clusters))
	for i := range clusters {
		clients[i], err = newClusterClient(clusters[i])
		if err != nil {
			return nil, err
		}
	}
	return clients, nil
}

func forEachCluster(fn func(client *clusterClient) error) error {
	clients, err := allClusters()
	if err != nil {
		return err
	}
	errors := tsuruErrors.NewMultiError()
	for _, c := range clients {
		err = fn(c)
		if err != nil {
			errors.Add(err)
		}
	}
	if errors.Len() > 0 {
		return errors
	}
	return nil
}
