// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tsuru

import (
	"errors"
	"fmt"
	"github.com/globocom/tsuru/git"
	"os"
	"regexp"
)

// AppGuesser is used to guess the name of an app based in a file path.
type AppGuesser interface {
	GuessName(path string) (string, error)
}

// GitGuesser uses git to guess the name of the app.
//
// It reads the "tsuru" remote from git config file. If the remote does not
// exist, or does not match the tsuru pattern (git@<something>:<app-name>.git),
// GuessName will return an error.
type GitGuesser struct{}

func (g GitGuesser) GuessName(path string) (string, error) {
	repoPath, err := git.DiscoverRepositoryPath(path)
	if err != nil {
		return "", fmt.Errorf("Git repository not found: %s.", err)
	}
	repo, err := git.OpenRepository(repoPath)
	if err != nil {
		return "", err
	}
	remoteUrl, err := repo.GetRemoteUrl("tsuru")
	if err != nil {
		return "", errors.New("tsuru remote not declared.")
	}
	re := regexp.MustCompile(`^git@.*:(.*)\.git$`)
	matches := re.FindStringSubmatch(remoteUrl)
	if len(matches) < 2 {
		return "", fmt.Errorf(`"tsuru" remote did not match the pattern. Want something like git@<host>:<app-name>.git, got %s`, remoteUrl)
	}
	return matches[1], nil
}

// Embed this struct if you want your command to guess the name of the app.
type GuessingCommand struct {
	G AppGuesser
}

func (cmd *GuessingCommand) guesser() AppGuesser {
	if cmd.G == nil {
		cmd.G = GitGuesser{}
	}
	return cmd.G
}

func (cmd *GuessingCommand) Guess() (string, error) {
	if AppName != nil && *AppName != "" {
		return *AppName, nil
	}
	path, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("Unable to guess app name: %s.", err)
	}
	name, err := cmd.guesser().GuessName(path)
	if err != nil {
		return "", errors.New(`tsuru wasn't able to guess the name of the app.

Use the --app flag to specify the name of the app.`)
	}
	return name, nil
}
