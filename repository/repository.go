package repository

import (
	"fmt"
	"github.com/timeredbull/tsuru/api/unit"
)

const gitServer = "tsuru.plataformas.glb.com"

func Clone(app string, machine int) (err error) {
	u := unit.Unit{Name: app, Machine: machine}
	cmd := fmt.Sprintf("git clone %s /home/application/%s", GetUrl(app), app)
	_, err = u.Command(cmd)
	if err != nil {
		return
	}
	return
}

func GetUrl(app string) string {
	return fmt.Sprintf("git@%s:%s.git", gitServer, app)
}

func GetReadOnlyUrl(app string) string {
	return fmt.Sprintf("git://%s/%s.git", gitServer, app)
}
