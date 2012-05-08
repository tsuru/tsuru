package gitosis

import (
	"errors"
	"fmt"
	ini "github.com/kless/goconfig/config"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/log"
	"os"
	"os/exec"
	"path"
)

func AddProject(name string) error {
	err := config.ReadConfigFile("/etc/tsuru/tsuru.conf")
	if err != nil {
		log.Panic(err)
	}

	repoPath, err := config.GetString("git:gitosis-repo")
	if err != nil {
		log.Panic(err)
		return err
	}

	confPath := path.Join(repoPath, "gitosis.conf")
	c, err := ini.ReadDefault(confPath)
	if err != nil {
		log.Panic(err)
		return err
	}

	sName := fmt.Sprintf("group %s", name)
	ok := c.AddSection(sName)
	if !ok {
		errStr := fmt.Sprintf(`Could not add section "group %s" in gitosis.conf, section already exists!`, name)
		log.Panic(errStr)
		return errors.New(errStr)
	}

	err = c.WriteFile(confPath, 0744, "gitosis configuration file")
	if err != nil {
		log.Panic(err)
		return err
	}

	pwd := os.Getenv("PWD")
	os.Chdir(repoPath)

	commitMsg := fmt.Sprintf("Defining gitosis group for project %s", name)
	output, err := exec.Command("git", "add", ".").CombinedOutput()
	if err != nil {
		fmt.Println(string(output))
		log.Panic(output, err)
		return err
	}
	output, err = exec.Command("git", "commit", "-m", commitMsg).CombinedOutput()
	if err != nil {
		fmt.Println(string(output))
		log.Panic(output, err)
		return err
	}

	output, err = exec.Command("git", "push", "origin", "master").CombinedOutput()
	if err != nil {
		fmt.Println(string(output))
		log.Panic(output, err)
		return err
	}

	os.Chdir(pwd)

	return nil
}
