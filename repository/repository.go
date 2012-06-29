package repository

import (
	"fmt"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/log"
	"path"
)

func Clone(app string, machine int) ([]byte, error) {
	u := unit.Unit{Name: app, Machine: machine}
	cmd := fmt.Sprintf("git clone %s /home/application/current --depth 1", GetReadOnlyUrl(app))
	output, err := u.Command(cmd)
	log.Printf(`"git clone" output: %s`, string(output))
	if err != nil {
		return output, err
	}
	return output, nil
}

func Pull(app string, machine int) ([]byte, error) {
	u := unit.Unit{Name: app, Machine: machine}
	cmd := fmt.Sprintf("cd /home/application/current && git pull origin master")
	output, err := u.Command(cmd)
	log.Printf(`"git pull" output: %s`, string(output))
	if err != nil {
		return output, err
	}
	return output, nil
}

func CloneOrPull(app string, machine int) (string, error) {
	var output []byte
	output, err := Clone(app, machine)
	if err != nil {
		output, err = Pull(app, machine)
		if err != nil {
			return string(output), err
		}
	}
	return string(output), nil
}

func getGitServer() string {
	gitServer, err := config.GetString("git:server")
	if err != nil {
		panic(err)
	}
	return gitServer
}

func GetUrl(app string) string {
	return fmt.Sprintf("git@%s:%s.git", getGitServer(), app)
}

func GetReadOnlyUrl(app string) string {
	return fmt.Sprintf("git://%s/%s.git", getGitServer(), app)
}

func GetPath() (string, error) {
	return config.GetString("git:unit-repo")
}

func GetBarePath(app string) (p string, err error) {
	if p, err = config.GetString("git:bare-base"); err == nil {
		p = path.Join(p, app+".git")
	}
	return
}
