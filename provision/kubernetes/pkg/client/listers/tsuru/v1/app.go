// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Code generated by lister-gen. DO NOT EDIT.

package v1

import (
	v1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// AppLister helps list Apps.
type AppLister interface {
	// List lists all Apps in the indexer.
	List(selector labels.Selector) (ret []*v1.App, err error)
	// Apps returns an object that can list and get Apps.
	Apps(namespace string) AppNamespaceLister
	AppListerExpansion
}

// appLister implements the AppLister interface.
type appLister struct {
	indexer cache.Indexer
}

// NewAppLister returns a new AppLister.
func NewAppLister(indexer cache.Indexer) AppLister {
	return &appLister{indexer: indexer}
}

// List lists all Apps in the indexer.
func (s *appLister) List(selector labels.Selector) (ret []*v1.App, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.App))
	})
	return ret, err
}

// Apps returns an object that can list and get Apps.
func (s *appLister) Apps(namespace string) AppNamespaceLister {
	return appNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// AppNamespaceLister helps list and get Apps.
type AppNamespaceLister interface {
	// List lists all Apps in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1.App, err error)
	// Get retrieves the App from the indexer for a given namespace and name.
	Get(name string) (*v1.App, error)
	AppNamespaceListerExpansion
}

// appNamespaceLister implements the AppNamespaceLister
// interface.
type appNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all Apps in the indexer for a given namespace.
func (s appNamespaceLister) List(selector labels.Selector) (ret []*v1.App, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.App))
	})
	return ret, err
}

// Get retrieves the App from the indexer for a given namespace and name.
func (s appNamespaceLister) Get(name string) (*v1.App, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource("app"), name)
	}
	return obj.(*v1.App), nil
}
