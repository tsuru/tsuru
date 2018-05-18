// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package v1alpha1

import (
	v1alpha1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1alpha1"
	scheme "github.com/tsuru/tsuru/provision/kubernetes/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// AppsGetter has a method to return a AppInterface.
// A group's client should implement this interface.
type AppsGetter interface {
	Apps(namespace string) AppInterface
}

// AppInterface has methods to work with App resources.
type AppInterface interface {
	Create(*v1alpha1.App) (*v1alpha1.App, error)
	Update(*v1alpha1.App) (*v1alpha1.App, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.App, error)
	List(opts v1.ListOptions) (*v1alpha1.AppList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.App, err error)
	AppExpansion
}

// apps implements AppInterface
type apps struct {
	client rest.Interface
	ns     string
}

// newApps returns a Apps
func newApps(c *TsuruV1alpha1Client, namespace string) *apps {
	return &apps{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the app, and returns the corresponding app object, and an error if there is any.
func (c *apps) Get(name string, options v1.GetOptions) (result *v1alpha1.App, err error) {
	result = &v1alpha1.App{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("apps").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of Apps that match those selectors.
func (c *apps) List(opts v1.ListOptions) (result *v1alpha1.AppList, err error) {
	result = &v1alpha1.AppList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("apps").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested apps.
func (c *apps) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("apps").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a app and creates it.  Returns the server's representation of the app, and an error, if there is any.
func (c *apps) Create(app *v1alpha1.App) (result *v1alpha1.App, err error) {
	result = &v1alpha1.App{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("apps").
		Body(app).
		Do().
		Into(result)
	return
}

// Update takes the representation of a app and updates it. Returns the server's representation of the app, and an error, if there is any.
func (c *apps) Update(app *v1alpha1.App) (result *v1alpha1.App, err error) {
	result = &v1alpha1.App{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("apps").
		Name(app.Name).
		Body(app).
		Do().
		Into(result)
	return
}

// Delete takes name of the app and deletes it. Returns an error if one occurs.
func (c *apps) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("apps").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *apps) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("apps").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched app.
func (c *apps) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.App, err error) {
	result = &v1alpha1.App{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("apps").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
