// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"

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

func convertToken(code, redirectUrl string) (string, error) {
	var token string
	v := url.Values{}
	v.Set("code", code)
	v.Set("redirectUrl", redirectUrl)
	u, err := GetURL("/auth/login")
	if err != nil {
		return token, fmt.Errorf("Error in GetURL: %s", err.Error())
	}
	resp, err := tsuruNet.Dial5Full300Client.Post(u, "application/x-www-form-urlencoded", strings.NewReader(v.Encode()))
	if err != nil {
		return token, fmt.Errorf("Error during login post: %s", err.Error())
	}
	defer resp.Body.Close()
	result, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return token, fmt.Errorf("Error reading body: %s", err.Error())
	}
	data := make(map[string]interface{})
	err = json.Unmarshal(result, &data)
	if err != nil {
		return token, fmt.Errorf("Error parsing response: %s - %s", result, err.Error())
	}
	return data["token"].(string), nil
}

func callback(redirectUrl string, finish chan bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			finish <- true
		}()
		var page string
		token, err := convertToken(r.URL.Query().Get("code"), redirectUrl)
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
	redirectUrl := fmt.Sprintf("http://localhost:%s", port)
	authUrl := strings.Replace(schemeData["authorizeUrl"], "__redirect_url__", redirectUrl, 1)
	http.HandleFunc("/", callback(redirectUrl, finish))
	server := &http.Server{}
	go server.Serve(l)
	err = open(authUrl)
	if err != nil {
		fmt.Fprintln(context.Stdout, "Failed to start your browser.")
		fmt.Fprintf(context.Stdout, "Please open the following URL in your browser: %s\n", authUrl)
	}
	<-finish
	fmt.Fprintln(context.Stdout, "Successfully logged in!")
	return nil
}
