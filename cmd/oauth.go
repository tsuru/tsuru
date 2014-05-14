// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/tsuru/tsuru/exec"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"runtime"
)

var execut exec.Executor

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
			return p
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
		params := url.Values{
			"code":        {"qwert"},
			"redirectUrl": {redirectUrl},
		}
		u, err := GetURL("/auth/login")
		if err != nil {
			return
		}
		resp, err := http.PostForm(u, params)
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
	}
}

func startServerAndOpenBrowser() {
	finish := make(chan bool)
	l, e := net.Listen("tcp", port())
	if e != nil {
		return
	}
	_, port, _ := net.SplitHostPort(l.Addr().String())
	redirectUrl := fmt.Sprintf(authorizeUrl(), fmt.Sprintf("http://localhost:%s", port))
	http.HandleFunc("/", callback(redirectUrl, finish))
	server := &http.Server{}
	go server.Serve(l)
	open(redirectUrl)
	<-finish
}

func oauthLogin(context *Context, client *Client) error {
	startServerAndOpenBrowser()
	return nil
}
