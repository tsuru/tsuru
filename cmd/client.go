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

type Doer interface {
	Do(request *http.Request) (*http.Response, error)
}

type Client struct {
	HttpClient     *http.Client
	context        *Context
	currentVersion string
	versionHeader  string
}

func NewClient(client *http.Client, context *Context, version, versionHeader string) *Client {
	return &Client{
		HttpClient:     client,
		context:        context,
		currentVersion: version,
		versionHeader:  versionHeader,
	}
}

func (c *Client) Do(request *http.Request) (*http.Response, error) {
	if token, err := readToken(); err == nil {
		request.Header.Set("Authorization", token)
	}
	response, err := c.HttpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to tsuru server (%s), it's probably down.", readTarget())
	}
	supported := response.Header.Get(c.versionHeader)
	format := `############################################################

WARNING: You're using an unsupported version of tsuru client.

You must have at least version %s, your current version is %s.

Please go to https://github.com/globocom/tsuru/downloads and download the last version.

############################################################

`
	if !validateVersion(supported, c.currentVersion) {
		fmt.Fprintf(c.context.Stderr, format, supported, c.currentVersion)
	}
	if response.StatusCode > 399 {
		defer response.Body.Close()
		result, _ := ioutil.ReadAll(response.Body)
		return nil, errors.New(string(result))
	}
	return response, nil
}
