// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/cmd/tsuru-base"
	"launchpad.net/gnuflag"
	"net/http"
	"strings"
)

type changeQuota struct {
	tsuru.GuessingCommand
	fs    *gnuflag.FlagSet
	quota int
	owner string
}

func (c *changeQuota) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "quota-update",
		Usage:   "quota-update [--owner/-o owner's name] [--quota/-q number of quotas]",
		Desc:    `Update quotas.`,
		MinArgs: 0,
	}
}

func (c *changeQuota) Run(context *cmd.Context, client *cmd.Client) error {
	if c.owner == "" {
		return errors.New("The owner's name required.")
	}
	uri := "/quota/" + c.owner
	url, err := cmd.GetURL(uri)
	if err != nil {
		return err
	}
	if c.quota == 0 {
		return errors.New("Number of quotas required.")
	}
	body := fmt.Sprintf("quota=%d", c.quota)
	request, err := http.NewRequest("PUT", url, strings.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintf(context.Stdout, "Quotas successfully changed!\n")
	return nil
}

func (c *changeQuota) Flags() *gnuflag.FlagSet {
	if c.fs == nil {
		c.fs = gnuflag.NewFlagSet("quota-update", gnuflag.ExitOnError)
		c.fs.IntVar(&c.quota, "quota", 0, "The number of quotas changed")
		c.fs.IntVar(&c.quota, "q", 0, "The number of quotas changed")
		c.fs.StringVar(&c.owner, "owner", "", "The owner will have quotas changed")
		c.fs.StringVar(&c.owner, "o", "", "The owner will have quotas changed")
	}
	return c.fs
}
