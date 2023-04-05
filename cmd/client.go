// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
	tsuruerr "github.com/tsuru/tsuru/errors"
	tsuruio "github.com/tsuru/tsuru/io"
)

const (
	VerbosityHeader = "X-Tsuru-Verbosity"
)

var errUnauthorized = &tsuruerr.HTTP{Code: http.StatusUnauthorized, Message: "unauthorized"}

type statusCoder interface {
	StatusCode() int
}

func isUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	if err == errUnauthorized {
		return true
	}
	if statusErr, ok := err.(statusCoder); ok {
		return statusErr.StatusCode() == http.StatusUnauthorized
	}
	return false
}

type Client struct {
	HTTPClient     *http.Client
	context        *Context
	progname       string
	currentVersion string
	versionHeader  string
	Verbosity      int
}

func NewClient(client *http.Client, context *Context, manager *Manager) *Client {
	w := io.Discard
	if context != nil && context.Stdout != nil {
		w = context.Stdout
	}
	cli := &Client{
		context:        context,
		progname:       manager.name,
		currentVersion: manager.version,
		versionHeader:  manager.versionHeader,
	}
	client.Transport = &VerboseRoundTripper{
		RoundTripper: client.Transport,
		Writer:       w,
		Verbosity:    &cli.Verbosity,
	}
	cli.HTTPClient = client
	return cli
}

func (c *Client) detectClientError(err error) error {
	urlErr, ok := err.(*url.Error)
	if !ok {
		return err
	}
	switch urlErr.Err.(type) {
	case x509.UnknownAuthorityError:
		target, _ := ReadTarget()
		return errors.Wrapf(urlErr.Err, "Failed to connect to tsuru server (%s)", target)
	}
	target, _ := ReadTarget()
	return errors.Errorf("Failed to connect to tsuru server (%s), it's probably down.", target)
}

func (c *Client) Do(request *http.Request) (*http.Response, error) {
	if token, err := ReadToken(); err == nil && token != "" {
		request.Header.Set("Authorization", "bearer "+token)
	}
	response, err := c.HTTPClient.Do(request)
	err = c.detectClientError(err)
	if err != nil {
		return nil, err
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
		err := &tsuruerr.HTTP{
			Code:    response.StatusCode,
			Message: response.Status,
		}

		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if len(body) > 0 {
			err.Message = string(body)
		}

		return response, err
	}
	return response, nil
}

// StreamJSONResponse supports the JSON streaming format from the tsuru API.
func StreamJSONResponse(w io.Writer, response *http.Response) error {
	if response == nil {
		return errors.New("response cannot be nil")
	}
	defer response.Body.Close()
	var err error
	output := tsuruio.NewStreamWriter(w, nil)
	for n := int64(1); n > 0 && err == nil; n, err = io.Copy(output, response.Body) {
	}
	if err != nil {
		return err
	}
	unparsed := output.Remaining()
	if len(unparsed) > 0 {
		return errors.Errorf("unparsed message error: %s", string(unparsed))
	}
	return nil
}
