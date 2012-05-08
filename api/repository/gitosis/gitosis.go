package gitosis

import (
	"errors"
	"fmt"
	ini "github.com/kless/goconfig/config"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/log"
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

	return nil
}
