package repository

import (
	"fmt"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/log"
)

const gitServer = "tsuru.plataformas.glb.com"

func CloneRepository(appName string) (err error) {
	u := unit.Unit{Name: appName}
	cmd := fmt.Sprintf("git clone %s /home/application/%s", GetRepositoryUrl(appName), appName)
	output, err := u.Command(cmd)
	if err != nil {
		log.Print(err)
		return
	}
	log.Print(output)
	return
}

func GetRepositoryUrl(appName string) string {
	return fmt.Sprintf("git@%s:%s.git", gitServer, appName)
}
