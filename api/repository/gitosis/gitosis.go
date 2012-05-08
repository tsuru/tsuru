package gitosis

import (
	"fmt"
	"path"
	ini "github.com/kless/goconfig/config"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/log"
)

func AddProject(name string) error {
	err := config.ReadConfigFile("/etc/tsuru/tsuru.conf")
	if err != nil {
		log.Panic(err)
	}

	repoPath, err := config.GetString("git:gitosis-repo")
	if err != nil {
		log.Panic(err)
	}

	confPath := path.Join(repoPath, "gitosis.conf")
	c, err := ini.ReadDefault(confPath)
	if err != nil {
		log.Panic(err)
	}

	sName := fmt.Sprintf("group %s", name)
	ok := c.AddSection(sName)
	if !ok {
		log.Panic(fmt.Sprintf(`Could not add section "group %s" in gitosis.conf`, name))
	}
	err = c.WriteFile(confPath, 0744, "gitosis configuration file")
	return nil
}
