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
	c, err := ini.Read(confPath, ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
	if err != nil {
		log.Print(err)
		return err
	}
	section := fmt.Sprintf("group %s", group) //check if session exists
	if !c.HasSection(section) {
		errMsg := fmt.Sprintf("Section %s doesn't exists", section)
		return errors.New(errMsg)
	}
	err = addOption(c, section, "writable", project)
	if err != nil {
		log.Print(err)
		return err
	}
	commit := fmt.Sprintf("Added project %s to group %s", project, group)
	err = writeCommitPush(c, commit)
	return nil
}

// Add a new group to gitosis.conf. Also commit and push changes.
func AddGroup(name string) error {
	c, err := getConfig()
	if err != nil {
		return err
	}
	sName := fmt.Sprintf("group %s", name)
	ok := c.AddSection(sName)
	if !ok {
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
	ok := c.RemoveSection(gName)
	if !ok {
		return errors.New("Section does not exists")
	}
	commitMsg := fmt.Sprintf("Removing group %s from gitosis.conf", group)
	return writeCommitPush(c, commitMsg)
}

// HasGroup checks if gitosis has the given group.
func HasGroup(group string) bool {
	c, err := getConfig()
	if err != nil {
		return false
	}
	return c.HasSection("group " + group)
}

// AddMember adds a member to the given group.
//
// It is up to the caller make sure that the member does
// have a key in the keydir, otherwise the member will not
// be able to push to the repository.
func AddMember(group, member string) error {
	c, err := getConfig()
	if err != nil {
		return err
	}
	section := fmt.Sprintf("group %s", group)
	if !c.HasSection(section) {
		return errors.New("Group not found")
	}
	err = addOption(c, section, "members", member)
	if err != nil {
		log.Print(err)
		return err
	}
	commitMsg := fmt.Sprintf("Adding member %s to group %s", member, group)
	return writeCommitPush(c, commitMsg)
}

// RemoveMember removes a member from the given group.
//
// It is up to the caller to delete the keyfile from the keydir
// using the DeleteKeyFile function.
func RemoveMember(group, member string) error {
	c, err := getConfig()
	if err != nil {
		return err
	}
	section := fmt.Sprintf("group %s", group)
	if !c.HasSection(section) {
		return errors.New("Group not found")
	}
	strMembers, err := c.String(section, "members")
	if err != nil {
		return errors.New("This group does not have any members")
	}
	members := strings.Split(strMembers, " ")
	index := find(members, member)
	if index < 0 {
		return errors.New("This group does not have this member")
	}
	last := len(members) - 1
	members[index] = members[last]
	members = members[:last]
	if len(members) > 0 {
		c.AddOption(section, "members", strings.Join(members, " "))
	} else {
		c.RemoveOption(section, "members")
	}
	commitMsg := fmt.Sprintf("Removing member %s from group %s", member, group)
	return writeCommitPush(c, commitMsg)
}

func addOption(c *ini.Config, section, option, value string) (err error) {
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
	optValues := strings.Trim(strings.Join(values, " "), " ")
	c.AddOption(section, option, optValues)
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
	return ini.Read(confPath, ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
}
