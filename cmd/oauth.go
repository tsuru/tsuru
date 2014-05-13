// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"github.com/tsuru/tsuru/exec"
	"net"
	"net/http"
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

func serve(url chan string, finish chan bool) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			finish <- true
		}()
		fmt.Fprintf(w, "Test")
	})
	l, e := net.Listen("tcp", port())
	if e != nil {
		return
	}
	_, port, _ := net.SplitHostPort(l.Addr().String())
	url <- fmt.Sprintf(authorizeUrl(), port)
	server := &http.Server{}
	server.Serve(l)
}

func startServerAndOpenBrowser() {
	url := make(chan string)
	finish := make(chan bool)
	go func() {
		serve(url, finish)
	}()
	open(<-url)
	<-finish
}

func oauthLogin(context *Context, client *Client) error {
	startServerAndOpenBrowser()
	return nil
}
