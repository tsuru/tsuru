// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tsuru/tsuru/exec"
	"io/ioutil"
	"net"
	"net/http"
	"runtime"
	"strings"
)

var execut exec.Executor

const callbackPage = `
<html>
<head>
	<script>window.close();</script>
	<style>
	body {
		text-align: center;
	}
	</style>
</head>
<body>
	<h1>Login Successful!</h1>
	<p>You can close this window now.</p>
</body>
</html>
`

func executor() exec.Executor {
	if execut == nil {
		execut = exec.OsExecutor{}
	}
	return execut
}

func authorizeUrl() string {
	info, err := schemeInfo()
	if err == nil {
		data := info["data"].(map[string]interface{})
		url := data["authorizeUrl"].(string)
		if url != "" {
			return url
		}
	}
	return ""
}

func port() string {
	info, err := schemeInfo()
	if err == nil {
		data := info["data"].(map[string]interface{})
		p := data["port"].(string)
		if p != "" {
			return fmt.Sprintf(":%s", p)
		}
	}
	return ":0"
}

func open(url string) error {
	if runtime.GOOS == "linux" {
		return executor().Execute("xdg-open", []string{url}, nil, nil, nil)
	}
	return executor().Execute("open", []string{url}, nil, nil, nil)
}

func callback(redirectUrl string, finish chan bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			finish <- true
		}()
		code := r.URL.Query().Get("code")
		params := map[string]string{"code": code, "redirectUrl": redirectUrl}
		u, err := GetURL("/auth/login")
		if err != nil {
			return
		}
		var buf bytes.Buffer
		err = json.NewEncoder(&buf).Encode(params)
		if err != nil {
			return
		}
		resp, err := http.Post(u, "application/json", &buf)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		result, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}
		data := make(map[string]interface{})
		err = json.Unmarshal(result, &data)
		if err != nil {
			return
		}
		writeToken(data["token"].(string))
		w.Header().Add("Content-Type", "text/html")
		w.Write([]byte(callbackPage))
	}
}

func oauthLogin(context *Context, client *Client) error {
	finish := make(chan bool)
	l, err := net.Listen("tcp", port())
	if err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return err
	}
	redirectUrl := fmt.Sprintf("http://localhost:%s", port)
	authUrl := strings.Replace(authorizeUrl(), "__redirect_url__", redirectUrl, 1)
	http.HandleFunc("/", callback(redirectUrl, finish))
	server := &http.Server{}
	go server.Serve(l)
	open(authUrl)
	<-finish
	return nil
}
