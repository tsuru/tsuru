package gitosis

import (
	"errors"
	"fmt"
	ini "github.com/kless/goconfig/config"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/log"
	"path"
	"strings"
)

// Add a new project to gitosis.conf.
func AddProject(group, project string) error {
	confPath, err := ConfPath()
	if err != nil {
		log.Print(err)
		return err
	}
	c, err := ini.ReadDefault(confPath)
	if err != nil {
		log.Print(err)
		return err
	}
	section := fmt.Sprintf("group %s", group) //check if session exists
	if !c.HasSection(section) {
		errMsg := fmt.Sprintf("Section %s doesn't exists", section)
		return errors.New(errMsg)
	}
	if err = addOptionValue(c, section, "writable", project); err != nil {
		log.Print(err)
		return err
	}
	commit := fmt.Sprintf("Added project %s to group %s", project, group)
	err = writeCommitPush(c, commit)
	return nil
}

// Remove a project from gitosis.conf
func RemoveProject(group, project string) error {
	return nil
}

// Add a new group to gitosis.conf. Also commit and push changes.
func AddGroup(name string) error {
	c, err := getConfig()
	if err != nil {
		return err
	}
	sName := fmt.Sprintf("group %s", name)
	if !c.AddSection(sName) {
		errStr := fmt.Sprintf(`Could not add section "group %s" in gitosis.conf, section already exists!`, name)
		return errors.New(errStr)
	}
	commitMsg := fmt.Sprintf("Defining gitosis group for group %s", name)
	return writeCommitPush(c, commitMsg)
}

// Removes a group section and all it's options.
func RemoveGroup(group string) error {
	c, err := getConfig()
	if err != nil {
		return err
	}
	gName := fmt.Sprintf("group %s", group)
	if !c.RemoveSection(gName) {
		return errors.New("Section does not exists")
	}
	commitMsg := fmt.Sprintf("Removing group %s from gitosis.conf", group)
	return writeCommitPush(c, commitMsg)
}

// addMember adds a member to the given group.
// member parameter should be the same as the key name in keydir dir.
func addMember(group, member string) error {
	c, err := getConfig()
	if err != nil {
		return err
	}
	section := fmt.Sprintf("group %s", group)
	if !c.HasSection(section) {
		return errors.New("Group not found")
	}
	if err = addOptionValue(c, section, "members", member); err != nil {
		log.Print(err)
		return err
	}
	commitMsg := fmt.Sprintf("Adding member %s to group %s", member, group)
	return writeCommitPush(c, commitMsg)
}

// removeMember removes a member from the given group.
func removeMember(group, member string) error {
	c, err := getConfig()
	if err != nil {
		return err
	}
	section := fmt.Sprintf("group %s", group)
	if !c.HasSection(section) {
		return errors.New("Group not found")
	}
	if !c.HasOption(section, "members") {
		return errors.New("This group does not have any members")
	}
	err = removeOptionValue(c, section, "members", member)
	if err != nil {
		return err
	}
	commitMsg := fmt.Sprintf("Removing member %s from group %s", member, group)
	return writeCommitPush(c, commitMsg)
}

func addOptionValue(c *ini.Config, section, option, value string) (err error) {
	var strValues string
	if c.HasOption(section, option) {
		strValues, err = c.String(section, option)
		if err != nil {
			log.Print(err)
			return err
		}
	}
	values := strings.Split(strValues, " ")
	if checkPresenceOfString(values, value) {
		errStr := fmt.Sprintf("Value %s for option %s in section %s has already been added", value, option, section)
		return errors.New(errStr)
	}
	values = append(values, value)
	optValues := strings.TrimSpace(strings.Join(values, " "))
	c.AddOption(section, option, optValues)
	return nil
}

func removeOptionValue(c *ini.Config, section, option, value string) (err error) {
	//check if section and option exists
	strValues, err := c.String(section, option)
	if err != nil {
		log.Print(err)
		return err
	}
	values := strings.Split(strValues, " ")
	index := find(values, value)
	if index < 0 {
		return errors.New(fmt.Sprintf("Value %s not found in section %s", value, section))
	}
	last := len(values) - 1
	values[index] = values[last]
	values = values[:last]
	if len(values) > 0 {
		c.AddOption(section, option, strings.Join(values, " "))
	} else {
		c.RemoveOption(section, option)
	}

	return nil
}

func ConfPath() (p string, err error) {
	p = ""
	repoPath, err := config.GetString("git:gitosis-repo")
	if err != nil {
		log.Print(err)
		return
	}
	p = path.Join(repoPath, "gitosis.conf")
	return
}

func find(strs []string, str string) int {
	for i, s := range strs {
		if str == s {
			return i
		}
	}
	return -1
}

func checkPresenceOfString(strs []string, str string) bool {
	return find(strs, str) > -1
}

func getConfig() (*ini.Config, error) {
	confPath, err := ConfPath()
	if err != nil {
		return nil, err
	}
	return ini.ReadDefault(confPath)
}
