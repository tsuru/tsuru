package repository

import (
	"fmt"
	"github.com/timeredbull/tsuru/api/unit"
)

const gitServer = "tsuru.plataformas.glb.com"

func Clone(appName string) (err error) {
	u := unit.Unit{Name: appName}
	cmd := fmt.Sprintf("git clone %s /home/application/%s", GetUrl(appName), appName)
	_, err = u.Command(cmd)
	if err != nil {
		return
	}
	return
}

func GetUrl(appName string) string {
	return fmt.Sprintf("git@%s:%s.git", gitServer, appName)
}
