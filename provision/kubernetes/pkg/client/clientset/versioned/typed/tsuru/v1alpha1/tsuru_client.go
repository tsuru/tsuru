// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package v1alpha1

import (
	v1alpha1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1alpha1"
	"github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned/scheme"
	serializer "k8s.io/apimachinery/pkg/runtime/serializer"
	rest "k8s.io/client-go/rest"
)

type TsuruV1alpha1Interface interface {
	RESTClient() rest.Interface
	AppsGetter
}

// TsuruV1alpha1Client is used to interact with features provided by the tsuru.io group.
type TsuruV1alpha1Client struct {
	restClient rest.Interface
}

func (c *TsuruV1alpha1Client) Apps(namespace string) AppInterface {
	return newApps(c, namespace)
}

// NewForConfig creates a new TsuruV1alpha1Client for the given config.
func NewForConfig(c *rest.Config) (*TsuruV1alpha1Client, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return &TsuruV1alpha1Client{client}, nil
}

// NewForConfigOrDie creates a new TsuruV1alpha1Client for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *TsuruV1alpha1Client {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new TsuruV1alpha1Client for the given RESTClient.
func New(c rest.Interface) *TsuruV1alpha1Client {
	return &TsuruV1alpha1Client{c}
}

func setConfigDefaults(config *rest.Config) error {
	gv := v1alpha1.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: scheme.Codecs}

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return nil
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *TsuruV1alpha1Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}
