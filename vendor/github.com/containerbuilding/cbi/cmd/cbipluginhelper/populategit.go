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
	"os"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v2"
)

var populateGitCommand = &cli.Command{
	Name:      "populate-git",
	Usage:     "populate git. Requires git and ssh to be installed.",
	ArgsUsage: "[flags] REPO-URL DIRECTORY",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "revision",
			Usage: "Revision. e.g. master",
		},
	},
	Action: populateGitAction,
}

func populateGitAction(clicontext *cli.Context) error {
	repoURL := clicontext.Args().Get(0)
	if repoURL == "" {
		return errors.New("REPOURL missing")
	}
	dir := clicontext.Args().Get(1)
	if dir == "" {
		return errors.New("DIRECTORY missing")
	}
	ctx := context.Background()
	if err := run(ctx, "git", "clone", repoURL, dir); err != nil {
		return err
	}
	os.Chdir(dir)
	if revision := clicontext.String("revision"); revision != "" {
		return run(ctx, "git", "checkout", revision)
	}
	return nil
}
