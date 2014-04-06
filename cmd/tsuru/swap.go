// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/tsuru/tsuru/cmd"
	"net/http"
)

type swap struct{}

func (swap) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "swap",
		Usage:   "swap app1-name app2-name",
		Desc:    "Swap router between two apps.",
		MinArgs: 2,
	}
}

func (swap) Run(context *cmd.Context, client *cmd.Client) error {
	url, err := cmd.GetURL(fmt.Sprintf("/swap?app1=%s&app2=%s", context.Args[0], context.Args[1]))
	if err != nil {
		return err
	}
	request, err := http.NewRequest("PUT", url, nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "Apps successfully swapped!")
	return nil
}
