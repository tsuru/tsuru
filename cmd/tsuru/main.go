// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/globocom/tsuru/cmd"
	"launchpad.net/gnuflag"
	"net/http"
	"os"
	"strings"
)

const version = "0.2"

var appname = gnuflag.String("app", "", "App name for running app related commands.")

func validateTsuruVersion(resp *http.Response, context *cmd.Context) {
	format := `You're using an unsupported version of tsuru client.

You must have at least version %s, your current version is %s.`
	var (
		bigger bool
		limit  int
	)
	supportedHeader := resp.Header.Get("Supported-Tsuru")
	partsSupported := strings.Split(supportedHeader, ".")
	partsCurrent := strings.Split(version, ".")
	if len(partsSupported) > len(partsCurrent) {
		limit = len(partsCurrent)
		bigger = true
	} else {
		limit = len(partsSupported)
	}
	for i := 0; i < limit; i++ {
		if partsCurrent[i] < partsSupported[i] {
			fmt.Fprintf(context.Stderr, format, supportedHeader, version)
			return
		}
	}
	if bigger {
		fmt.Fprintf(context.Stderr, format, supportedHeader, version)
	}
}

func buildManager(name string) *cmd.Manager {
	m := cmd.BuildBaseManager(name, version)
	m.Register(&AppRun{})
	m.Register(&AppInfo{})
	m.Register(&AppCreate{})
	m.Register(&AppRemove{})
	m.Register(&AppList{})
	m.Register(&AppLog{})
	m.Register(&AppGrant{})
	m.Register(&AppRevoke{})
	m.Register(&AppRestart{})
	m.Register(&EnvGet{})
	m.Register(&EnvSet{})
	m.Register(&EnvUnset{})
	m.Register(&KeyAdd{})
	m.Register(&KeyRemove{})
	m.Register(&ServiceList{})
	m.Register(&ServiceAdd{})
	m.Register(&ServiceRemove{})
	m.Register(&ServiceBind{})
	m.Register(&ServiceUnbind{})
	m.Register(&ServiceDoc{})
	m.Register(&ServiceInfo{})
	m.Register(&ServiceInstanceStatus{})
	return m
}

func main() {
	gnuflag.Parse(true)
	name := cmd.ExtractProgramName(os.Args[0])
	manager := buildManager(name)
	args := gnuflag.Args()
	manager.Run(args)
}
