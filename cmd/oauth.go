// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	stdcontext "context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/exec"
	tsuruNet "github.com/tsuru/tsuru/net"
)

var execut exec.Executor

const callbackPage = `<!DOCTYPE html>
<html>
<head>
	<style>
	body {
		text-align: center;
	}
	</style>
</head>
<body>
	%s
</body>
</html>
`

const successMarkup = `
	<script>window.close();</script>
	<h1>Login Successful!</h1>
	<p>You can close this window now.</p>
`

const errorMarkup = `
	<h1>Login Failed!</h1>
	<p>%s</p>
`

func executor() exec.Executor {
	if execut == nil {
		execut = exec.OsExecutor{}
	}
	return execut
}

func port(schemeData map[string]string) string {
	p := schemeData["port"]
	if p != "" {
		return fmt.Sprintf(":%s", p)
	}
	return ":0"
}

func convertToken(code, redirectURL string) (string, error) {
	var token string
	v := url.Values{}
	v.Set("code", code)
	v.Set("redirectUrl", redirectURL)
	u, err := GetURL("/auth/login")
	if err != nil {
		return token, errors.Wrap(err, "Error in GetURL")
	}
	resp, err := tsuruNet.Dial15Full300Client.Post(u, "application/x-www-form-urlencoded", strings.NewReader(v.Encode()))
	if err != nil {
		return token, errors.Wrap(err, "Error during login post")
	}
	defer resp.Body.Close()
	result, err := io.ReadAll(resp.Body)
	if err != nil {
		return token, errors.Wrap(err, "Error reading body")
	}
	data := make(map[string]interface{})
	err = json.Unmarshal(result, &data)
	if err != nil {
		return token, errors.Wrapf(err, "Error parsing response: %s", result)
	}
	return data["token"].(string), nil
}

func callback(redirectURL string, finish chan bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			finish <- true
		}()
		var page string
		token, err := convertToken(r.URL.Query().Get("code"), redirectURL)
		if err == nil {
			writeToken(token)
			page = fmt.Sprintf(callbackPage, successMarkup)
		} else {
			msg := fmt.Sprintf(errorMarkup, err.Error())
			page = fmt.Sprintf(callbackPage, msg)
		}
		w.Header().Add("Content-Type", "text/html")
		w.Write([]byte(page))
	}
}

func (c *login) oauthLogin(context *Context, client *Client) error {
	schemeData := c.getScheme().Data
	finish := make(chan bool)
	l, err := net.Listen("tcp", port(schemeData))
	if err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return err
	}
	redirectURL := fmt.Sprintf("http://localhost:%s", port)
	authURL := strings.Replace(schemeData["authorizeUrl"], "__redirect_url__", redirectURL, 1)
	http.HandleFunc("/", callback(redirectURL, finish))
	server := &http.Server{}
	go server.Serve(l)
	err = open(authURL)
	if err != nil {
		fmt.Fprintln(context.Stdout, "Failed to start your browser.")
		fmt.Fprintf(context.Stdout, "Please open the following URL in your browser: %s\n", authURL)
	}
	<-finish
	ctx, cancel := stdcontext.WithTimeout(stdcontext.Background(), 15*time.Second)
	defer cancel()
	server.Shutdown(ctx)
	fmt.Fprintln(context.Stdout, "Successfully logged in!")
	return nil
}
