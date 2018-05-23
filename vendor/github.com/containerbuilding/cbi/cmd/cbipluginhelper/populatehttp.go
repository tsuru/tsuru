/*
Copyright The CBI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"net/http"
	"os"
	"os/exec"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v2"
)

var populateHTTPCommand = &cli.Command{
	Name:      "populate-http",
	Usage:     "populate an tar archive via HTTP(S). Requires bsdtar to be installed (for auto-detecting gzip compression).",
	ArgsUsage: "[flags] URL DIRECTORY",
	Action:    populateHTTPAction,
}

func populateHTTPAction(clicontext *cli.Context) error {
	u := clicontext.Args().Get(0)
	if u == "" {
		return errors.New("URL missing")
	}
	dir := clicontext.Args().Get(1)
	if dir == "" {
		return errors.New("DIRECTORY missing")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	resp, err := http.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	ctx := context.Background()
	// busybox tar and GNU tar can auto-detect gzip files, but not gzip stream.
	// so we use bsdtar.
	// TODO: rewrite in pure Go.
	cmd := exec.CommandContext(ctx, "bsdtar", "Cxvf", dir, "-")
	cmd.Stdin = resp.Body
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
