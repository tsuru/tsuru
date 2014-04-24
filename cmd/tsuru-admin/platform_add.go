// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"errors"
	"github.com/tsuru/tsuru/cmd"
	"io"
	"launchpad.net/gnuflag"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

type platformAdd struct {
	name       string
	dockerfile string
	fs         *gnuflag.FlagSet
}

func (p *platformAdd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "platform-add",
		Usage:   "platform-add [platform name] [--dockerfile/-d Dockerfile]",
		Desc:    "Add new platform to tsuru.",
		MinArgs: 2,
	}
}

func (p *platformAdd) Run(context *cmd.Context, client *cmd.Client) error {
	name := context.Args[0]
	if name == "" {
		return errors.New("The platform's name required.")
	}

	dockerfile_path, err := filepath.Abs(p.dockerfile)
	if err != nil {
		return errors.New("The Dockerfile doens't exists.")
	}

	file, err := os.Open(dockerfile_path)
	if err != nil {
		return err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("dockerfile", dockerfile_path)
	if err != nil {
		return err
	}

	_, err = io.Copy(part, file)
	_ = writer.WriteField("name", name)

	err = writer.Close()
	if err != nil {
		return err
	}

	request, err := http.NewRequest("PUT", "/platform/add", body)
	if err != nil {
		return err
	}

	_, err = client.Do(request)
	if err != nil {
		return err
	}

	return nil
}

func (p *platformAdd) Flags() *gnuflag.FlagSet {
	if p.fs == nil {
		p.fs = gnuflag.NewFlagSet("platform-add", gnuflag.ExitOnError)
		p.fs.StringVar(&p.dockerfile, "dockerfile", "", "The dockerfile to create a platform")
		p.fs.StringVar(&p.dockerfile, "d", "", "The dockerfile to create a platform")
	}

	return p.fs
}
