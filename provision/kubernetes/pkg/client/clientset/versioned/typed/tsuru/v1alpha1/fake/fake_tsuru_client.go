// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package fake

import (
	v1alpha1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned/typed/tsuru/v1alpha1"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeTsuruV1alpha1 struct {
	*testing.Fake
}

func (c *FakeTsuruV1alpha1) Apps(namespace string) v1alpha1.AppInterface {
	return &FakeApps{c, namespace}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeTsuruV1alpha1) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}
