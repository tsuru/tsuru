package repository

import (
	"fmt"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/log"
	"path"
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
	Command(cmd string) ([]byte, error)
}

// Clone runs a git clone to clone the app repository in a unit.
//
// Given a machine id (from juju), it runs a git clone into this machine,
// cloning from the bare repository that is being served by git-daemon in the
// tsuru server.
func Clone(u Unit) ([]byte, error) {
	cmd := fmt.Sprintf("git clone %s /home/application/current --depth 1", GetReadOnlyUrl(u.GetName()))
	output, err := u.Command(cmd)
	log.Printf(`"git clone" output: %s`, string(output))
	if err != nil {
		return output, err
	}
	return output, nil
}

// Pull runs a git pull to update the code in a unit.
//
// It works like Clone, pulling from the app bare repository.
func Pull(u Unit) ([]byte, error) {
	cmd := fmt.Sprintf("cd /home/application/current && git pull origin master")
	output, err := u.Command(cmd)
	log.Printf(`"git pull" output: %s`, string(output))
	if err != nil {
		return output, err
	}
	return output, nil
}

// CloneOrPull runs a git clone or a git pull in a unit of the app.
//
// First it tries to clone, and if the clone fail (meaning that the repository
// is already cloned), it pulls changes from the bare repository.
func CloneOrPull(u Unit) (string, error) {
	var output []byte
	output, err := Clone(u)
	if err != nil {
		output, err = Pull(u)
		if err != nil {
			return string(output), err
		}
	}
	return string(output), nil
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
	return fmt.Sprintf("git@%s:%s.git", getGitServer(), app)
}

// GetReadOnlyUrl returns the ssh url for communication with git-daemon.
func GetReadOnlyUrl(app string) string {
	return fmt.Sprintf("git://%s/%s.git", getGitServer(), app)
}

// GetPath returns the path to the repository where the app code is in its
// units.
func GetPath() (string, error) {
	return config.GetString("git:unit-repo")
}

// GetBarePath returns the bare path for the app in the tsuru server.
func GetBarePath(app string) (p string, err error) {
	if p, err = config.GetString("git:root"); err == nil {
		p = path.Join(p, app+".git")
	}
	return
}
