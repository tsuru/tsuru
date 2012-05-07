package repository

import(
	"fmt"
	"github.com/timeredbull/tsuru/api/unit"
	"github.com/timeredbull/tsuru/log"
	"os"
	"os/exec"
	"path"
)

const gitServer = "tsuru.plataformas.glb.com"

func NewRepository(appName string) (err error) {
	repoPath := GetRepositoryPath(appName)

	err = os.Mkdir(repoPath, 0700)
	if err != nil {
		return
	}

	oldPwd, err := os.Getwd()
	if err != nil {
		return
	}

	err = os.Chdir(repoPath)
	if err != nil {
		return
	}

	err = exec.Command("git", "init", "--bare").Run()
	if err != nil {
		return
	}

	err = os.Chdir(oldPwd)
	return
}

func DeleteRepository(appName string) error {
	return os.RemoveAll(GetRepositoryPath(appName))
}

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

func GetRepositoryPath(appName string) string {
	home := os.Getenv("HOME")
	return path.Join(home, "../git", GetRepositoryName(appName))
}

func GetRepositoryUrl(appName string) string {
	return fmt.Sprintf("git@%s:%s", gitServer, GetRepositoryName(appName))
}

func GetRepositoryName(appName string) string {
	return fmt.Sprintf("%s.git", appName)
}
