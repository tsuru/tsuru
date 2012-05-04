package repository

import(
	"fmt"
	"github.com/timeredbull/tsuru/api/app"
	"github.com/timeredbull/tsuru/log"
	"os"
	"os/exec"
	"path"
)

func NewRepository(app *app.App) (err error) {
	repoPath := GetRepositoryPath(app)

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

func DeleteRepository(app *app.App) error {
	return os.RemoveAll(GetRepositoryPath(app))
}

func CloneRepository(app *app.App) (err error) {
	u := unit.Unit{Name: app.Name}
	cmd := fmt.Sprintf("git clone %s /home/application/%s", GetRepositoryUrl(app), app.Name)
	output, err := u.Command(cmd)
	if err != nil {
		log.Print(err)
		return
	}

	log.Print(output)
	return
}

func GetRepositoryPath(app *app.App) string {
	home := os.Getenv("HOME")
	return path.Join(home, "../git", GetRepositoryName(app))
}

func GetRepositoryUrl(app *app.App) string {
	return fmt.Sprintf("git@%s:%s", gitServer, GetRepositoryName(app))
}

func GetRepositoryName(app *app.App) string {
	return fmt.Sprintf("%s.git", app.Name)
}
