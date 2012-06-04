package repository

import (
	"fmt"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/log"
)

const gitServer = "tsuru.plataformas.glb.com"

func Clone(app string, machine int) ([]byte, error) {
	u := unit.Unit{Name: app, Machine: machine}
	cmd := fmt.Sprintf("git clone %s /home/application/current --depth 1", GetReadOnlyUrl(app))
	output, err := u.Command(cmd)
	log.Printf(`"git clone" output: ` + string(output))
	if err != nil {
		return output, err
	}
	return output, nil
}

func Pull(app string, machine int) ([]byte, error) {
	u := unit.Unit{Name: app, Machine: machine}
	cmd := fmt.Sprintf("git --git-dir=/home/application/current/.git --work-tree=/home/application/current pull origin master")
	output, err := u.Command(cmd)
	log.Printf(`"git pull" output: ` + string(output))
	if err != nil {
		return output, err
	}
	return output, nil
}

func CloneOrPull(app string, machine int) (string, error) {
	var output []byte
	output, err := Clone(app, machine)
	fmt.Println(string(output))
	if err != nil {
		output, err = Pull(app, machine)
		fmt.Println(string(output))
		if err != nil {
			return string(output), err
		}
	}
	return string(output), nil
}

func GetUrl(app string) string {
	return fmt.Sprintf("git@%s:%s.git", gitServer, app)
}

func GetReadOnlyUrl(app string) string {
	return fmt.Sprintf("git://%s/%s.git", gitServer, app)
}

func GetPath() (string, error) {
	unitRepo, err := config.GetString("git:unit-repo")
	return unitRepo, err
}
