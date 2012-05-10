package gitosis

import (
	"fmt"
	ini "github.com/kless/goconfig/config"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/log"
	"os"
	"os/exec"
	"path"
)

func getKeydirPath() (string, error) {
	repoPath, err := config.GetString("git:gitosis-repo")
	if err != nil {
		log.Print(err)
		return "", err
	}
	return path.Join(repoPath, "keydir"), nil
}

// Add, commit and push all changes in gitosis repository to it's
// bare.
func pushToGitosis(cMsg string) error {
	repoPath, err := config.GetString("git:gitosis-repo")
	if err != nil {
		log.Print(err)
		return err
	}
	pwd, err := os.Getwd()
	if err != nil {
		log.Print(err)
		return err
	}
	os.Chdir(repoPath)
	output, err := exec.Command("git", "add", ".").CombinedOutput()
	if err != nil {
		fmt.Println(string(output))
		log.Print(output, err)
		return err
	}
	output, err = exec.Command("git", "commit", "-am", cMsg).CombinedOutput()
	if err != nil {
		fmt.Println(string(output))
		log.Print(output, err)
		return err
	}
	output, err = exec.Command("git", "push", "origin", "master").CombinedOutput()
	if err != nil {
		fmt.Println(string(output))
		log.Print(output, err)
		return err
	}
	os.Chdir(pwd)
	return nil
}

func writeCommitPush(c *ini.Config, commit string) error {
	confPath, err := ConfPath()
	if err != nil {
		return err
	}
	err = c.WriteFile(confPath, 0744, "gitosis configuration file")
	if err != nil {
		return err
	}
	err = pushToGitosis(commit)
	if err != nil {
		log.Print(err)
		return err
	}
	return nil
}
