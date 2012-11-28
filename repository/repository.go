// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"bytes"
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/log"
	"io"
	"strings"
)

// Unit interface represents a unit of execution.
//
// It must provide two methods:
//
//   * GetName: returns the name of the unit.
//   * Command: runs a command in the unit.
//
// Whatever that has a name and is able to run commands, is a unit.
type Unit interface {
	GetName() string
	Command(stdout, stderr io.Writer, cmd ...string) error
}

// Clone runs a git clone to clone the app repository in a unit.
//
// Given a machine id (from juju), it runs a git clone into this machine,
// cloning from the bare repository that is being served by git-daemon in the
// tsuru server.
func clone(u Unit) ([]byte, error) {
	var buf bytes.Buffer
	cmd := fmt.Sprintf("git clone %s /home/application/current --depth 1", GetReadOnlyUrl(u.GetName()))
	err := u.Command(&buf, &buf, cmd)
	b := buf.Bytes()
	log.Printf(`"git clone" output: %s`, b)
	return b, err
}

// Pull runs a git pull to update the code in a unit.
//
// It works like Clone, pulling from the app bare repository.
func pull(u Unit) ([]byte, error) {
	var buf bytes.Buffer
	cmd := fmt.Sprintf("cd /home/application/current && git pull origin master")
	err := u.Command(&buf, &buf, cmd)
	b := buf.Bytes()
	log.Printf(`"git pull" output: %s`, b)
	return b, err
}

// CloneOrPull runs a git clone or a git pull in a unit of the app.
//
// First it tries to clone, and if the clone fail (meaning that the repository
// is already cloned), it pulls changes from the bare repository.
func CloneOrPull(u Unit) ([]byte, error) {
	b, err := clone(u)
	if err != nil {
		b, err = pull(u)
	}
	return b, err
}

// getGitServer returns the git server defined in the tsuru.conf file.
//
// If it is not defined, this function panics.
func getGitServer() string {
	gitServer, err := config.GetString("git:server")
	if err != nil {
		panic(err)
	}
	return gitServer
}

// GetUrl returns the ssh clone-url from an app.
func GetUrl(app string) string {
	s := strings.Replace(getGitServer(), "http://", "", -1) // https?
	return fmt.Sprintf("git@%s:%s.git", s, app)
}

// GetReadOnlyUrl returns the ssh url for communication with git-daemon.
func GetReadOnlyUrl(app string) string {
	s := strings.Replace(getGitServer(), "http://", "", -1) // https?
	return fmt.Sprintf("git://%s/%s.git", s, app)
}

// GetPath returns the path to the repository where the app code is in its
// units.
func GetPath() (string, error) {
	return config.GetString("git:unit-repo")
}
