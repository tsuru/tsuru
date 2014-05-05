// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/tsuru/tsuru/cmd"
	"io"
	"launchpad.net/gnuflag"
	"net/http"
	"strings"
)

type platformAdd struct {
	name       string
	dockerfile string
	fs         *gnuflag.FlagSet
}

func (p *platformAdd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "platform-add",
		Usage:   "platform-add <platform name> [--dockerfile/-d Dockerfile]",
		Desc:    "Add new platform to tsuru.",
		MinArgs: 1,
	}
}

func (p *platformAdd) Run(context *cmd.Context, client *cmd.Client) error {
	name := context.Args[0]
	body := fmt.Sprintf("name=%s&dockerfile=%s", name, p.dockerfile)
	url, err := cmd.GetURL("/platforms/add")
	request, err := http.NewRequest("PUT", url, strings.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
    for n := int64(1); n > 0 && err == nil; n, err = io.Copy(context.Stdout, response.Body) {
	}
	fmt.Fprintf(context.Stdout, "Platform successfully added!\n")
	return nil
}

func (p *platformAdd) Flags() *gnuflag.FlagSet {
	message := "The dockerfile url to create a platform"
	if p.fs == nil {
		p.fs = gnuflag.NewFlagSet("platform-add", gnuflag.ExitOnError)
		p.fs.StringVar(&p.dockerfile, "dockerfile", "", message)
		p.fs.StringVar(&p.dockerfile, "d", "", message)
	}
	return p.fs
}

type platformUpdate struct {
    name string
    dockerfile string
    forceUpdate bool
    fs *gnuflag.FlagSet
}

func (p *platformUpdate) Info() *cmd.Info {
    return &cmd.Info{
        Name:    "platform-update",
        Usage:   "platform-update <platform name> [--dockerfile/-d Dockerfile --force-update=true]",
        Desc:    "Update a platform to tsuru.",
        MinArgs: 1,
    }
}

func (p *platformUpdate) Flags() *gnuflag.FlagSet {
    forceUpdateMessage := "Force apps to update your platform in next deploy"
    dockerfileMessage := "The dockerfile url to update a platform"
    if p.fs == nil {
        p.fs = gnuflag.NewFlagSet("platform-update", gnuflag.ExitOnError)
        p.fs.StringVar(&p.dockerfile, "dockerfile", "", dockerfileMessage)
        p.fs.StringVar(&p.dockerfile, "d", "", dockerfileMessage)
        p.fs.BoolVar(&p.forceUpdate, "force-update", false, forceUpdateMessage)
    }
    return p.fs
}

func (p *platformUpdate) Run(context *cmd.Context, client *cmd.Client) error {
    name := context.Args[0]
    body := fmt.Sprintf("name=%s&dockerfile=%s&forceUpdate=%t", name, p.dockerfile, p.forceUpdate)
    url, err := cmd.GetURL("/platforms/update")
    request, err := http.NewRequest("PUT", url, strings.NewReader(body))
    if err != nil {
        return err
    }
    request.Header.Add("Content-Type", "application/x-www-form-urlencoded")
    response, err := client.Do(request)
    if err != nil {
        return err
    }
    defer response.Body.Close()
    for n := int64(1); n > 0 && err == nil; n, err = io.Copy(context.Stdout, response.Body) {
    }
    fmt.Fprintf(context.Stdout, "Platform successfully updated!\n")
    return nil
}
