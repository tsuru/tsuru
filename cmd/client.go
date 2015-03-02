// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/tsuru/tsuru/errors"
)

var errUnauthorized = &errors.HTTP{Code: http.StatusUnauthorized, Message: "unauthorized"}

type Client struct {
	HTTPClient     *http.Client
	context        *Context
	progname       string
	currentVersion string
	versionHeader  string
	Verbosity      int
}

func NewClient(client *http.Client, context *Context, manager *Manager) *Client {
	return &Client{
		HTTPClient:     client,
		context:        context,
		progname:       manager.name,
		currentVersion: manager.version,
		versionHeader:  manager.versionHeader,
	}
}

func (c *Client) detectClientError(err error) error {
	urlErr, ok := err.(*url.Error)
	if !ok {
		return err
	}
	switch urlErr.Err.(type) {
	case x509.UnknownAuthorityError:
		target, _ := ReadTarget()
		return fmt.Errorf("Failed to connect to tsuru server (%s): %s", target, urlErr.Err)
	}
	target, _ := ReadTarget()
	return fmt.Errorf("Failed to connect to tsuru server (%s), it's probably down.", target)
}

func (c *Client) Do(request *http.Request) (*http.Response, error) {
	if token, err := ReadToken(); err == nil {
		request.Header.Set("Authorization", "bearer "+token)
	}
	request.Close = true
	if c.Verbosity >= 1 {
		fmt.Fprintf(c.context.Stdout, "*************************** <Request uri=%q> **********************************\n", request.URL.RequestURI())
		requestDump, err := httputil.DumpRequest(request, true)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(c.context.Stdout, string(requestDump))
		if requestDump[len(requestDump)-1] != '\n' {
			fmt.Fprintln(c.context.Stdout)
		}
		fmt.Fprintf(c.context.Stdout, "*************************** </Request uri=%q> **********************************\n", request.URL.RequestURI())
	}
	response, err := c.HTTPClient.Do(request)
	err = c.detectClientError(err)
	if err != nil {
		return nil, err
	}
	if c.Verbosity >= 2 {
		fmt.Fprintf(c.context.Stdout, "*************************** <Response uri=%q> **********************************\n", request.URL.RequestURI())
		responseDump, err := httputil.DumpResponse(response, true)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(c.context.Stdout, string(responseDump))
		if responseDump[len(responseDump)-1] != '\n' {
			fmt.Fprintln(c.context.Stdout)
		}
		fmt.Fprintf(c.context.Stdout, "*************************** </Response uri=%q> **********************************\n", request.URL.RequestURI())
	}
	supported := response.Header.Get(c.versionHeader)
	format := `#####################################################################

WARNING: You're using an unsupported version of %s.

You must have at least version %s, your current
version is %s.

Please go to http://docs.tsuru.io/en/latest/using/install-client.html
and download the last version.

#####################################################################

`
	if !validateVersion(supported, c.currentVersion) {
		fmt.Fprintf(c.context.Stderr, format, c.progname, supported, c.currentVersion)
	}
	if response.StatusCode == http.StatusUnauthorized {
		return response, errUnauthorized
	}
	if response.StatusCode > 399 {
		defer response.Body.Close()
		result, _ := ioutil.ReadAll(response.Body)
		err := &errors.HTTP{
			Code:    response.StatusCode,
			Message: string(result),
		}
		return response, err
	}
	return response, nil
}
