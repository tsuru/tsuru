// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
)

type Callback func(resp *http.Response, context *Context)

type Doer interface {
	Do(*http.Request, *Context) (*http.Response, error)
	RegisterCallback(Callback)
}

type Client struct {
	HttpClient *http.Client
	callbacks  []Callback
}

func NewClient(client *http.Client) *Client {
	return &Client{HttpClient: client}
}

func (c *Client) Do(request *http.Request, context *Context) (*http.Response, error) {
	if token, err := readToken(); err == nil {
		request.Header.Set("Authorization", token)
	}
	response, err := c.HttpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to tsuru server (%s), it's probably down.", readTarget())
	}
	for _, callback := range c.callbacks {
		callback(response, context)
	}
	if response.StatusCode > 399 {
		defer response.Body.Close()
		result, _ := ioutil.ReadAll(response.Body)
		return nil, errors.New(string(result))
	}
	return response, nil
}

func (c *Client) RegisterCallback(callback Callback) {
	c.callbacks = append(c.callbacks, callback)
}
