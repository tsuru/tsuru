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
	"strings"
)

func checkPresenceOfString(strs []string, str string) bool {
	for _, s := range strs {
		if str == s {
			return true
		}
	}
	return false
}

// Add a new group to gitosis.conf. Also commit and push changes.
func AddGroup(name string) error {
	confPath, err := ConfPath()
	if err != nil {
		log.Panic(err)
		return err
	}
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

	commitMsg := fmt.Sprintf("Defining gitosis group for group %s", name)
	err = PushToGitosis(commitMsg)
	if err != nil {
		log.Panic(err)
		return err
	}

	return nil
}

// Removes a group section and all it's options.
func RemoveGroup(group string) error {
	confPath, err := ConfPath()
	if err != nil {
		log.Panic(err)
		return err
	}
	c, err := ini.ReadDefault(confPath)
	if err != nil {
		log.Panic(err)
		return err
	}

	gName := fmt.Sprintf("group %s", group)
	ok := c.RemoveSection(gName)
	if !ok {
		log.Panic("Section does not exists")
		return errors.New("Section does not exists")
	}

	err = c.WriteFile(confPath, 0744, "gitosis configuration file")
	if err != nil {
		log.Panic(err)
		return err
	}

	commitMsg := fmt.Sprintf("Removing group %s from gitosis.conf", group)
	err = PushToGitosis(commitMsg)
	if err != nil {
		log.Panic(err)
		return err
	}

	return nil
}

// Adds a member to the given group.
// member parameter should be the same as the key name in keydir dir.
func AddMember(group, member string) error {
	confPath, err := ConfPath()
	if err != nil {
		log.Panic(err)
		return err
	}
	c, err := ini.ReadDefault(confPath)
	if err != nil {
		log.Panic(err)
		return err
	}
	section := fmt.Sprintf("group %s", group)
	if !c.HasSection(section) {
		return errors.New("Group not found")
	}
	var members []string
	if strMembers, err := c.String(section, "members"); err == nil {
		members = strings.Split(strMembers, " ")
	}
	if checkPresenceOfString(members, member) {
		return errors.New("This user is already member of this group")
	}
	members = append(members, member)
	c.AddOption(section, "members", strings.Join(members, " "))
	err = c.WriteFile(confPath, 0744, "gitosis configuration file")
	if err != nil {
		log.Panic(err)
		return err
	}
	c, err = ini.ReadDefault(confPath)
	commitMsg := fmt.Sprintf("Adding member %s to group %s", member, group)
	err = PushToGitosis(commitMsg)
	if err != nil {
		log.Panic(err)
		return err
	}
	return nil
}

// Add, commit and push all changes in gitosis repository to it's
// bare.
func PushToGitosis(cMsg string) error {
	repoPath, err := config.GetString("git:gitosis-repo")
	if err != nil {
		log.Panic(err)
		return err
	}

	pwd := os.Getenv("PWD")
	os.Chdir(repoPath)

	output, err := exec.Command("git", "add", ".").CombinedOutput()
	if err != nil {
		fmt.Println(string(output))
		log.Panic(output, err)
		return err
	}
	output, err = exec.Command("git", "commit", "-m", cMsg).CombinedOutput()
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

func ConfPath() (p string, err error) {
	p = ""
	repoPath, err := config.GetString("git:gitosis-repo")
	if err != nil {
		log.Panic(err)
		return
	}

	p = path.Join(repoPath, "gitosis.conf")
	return
}
