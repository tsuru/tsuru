// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"errors"
	"fmt"
	"github.com/globocom/config"
	ini "github.com/kless/goconfig/config"
	"os"
	"path"
	"strings"
	"sync"
	"syscall"
)

// gitosisManager is the gitosis implementation of the manager interface.
type gitosisManager struct {
	confPath string
	git      *repository
	sync.RWMutex
}

// newGitosisManager returns a new instance of gitosis manager.
//
// It uses some information from config file, make sure configuration is loaded
// before call this function.
func newGitosisManager() (*gitosisManager, error) {
	repoPath, err := config.GetString("git:gitosis-repo")
	if err != nil {
		return nil, err
	}
	manager := &gitosisManager{
		confPath: path.Join(repoPath, "gitosis.conf"),
		git:      &repository{path: repoPath},
	}
	return manager, nil
}

// addProject adds a new project to a group in gitosis.conf.
func (m *gitosisManager) addProject(group, project string) error {
	c, err := m.getConfig()
	if err != nil {
		return err
	}
	section := fmt.Sprintf("group %s", group) //check if session exists
	if !c.HasSection(section) {
		errMsg := fmt.Sprintf("Section %s doesn't exists", section)
		return errors.New(errMsg)
	}
	if err = addOptionValue(c, section, "writable", project); err != nil {
		return err
	}
	commit := fmt.Sprintf("Added project %s to group %s", project, group)
	err = m.writeCommitPush(c, commit)
	return nil
}

// removeProject removes a project from a group in gitosis.conf
func (m *gitosisManager) removeProject(group, project string) error {
	c, err := m.getConfig()
	if err != nil {
		return err
	}
	section := fmt.Sprintf("group %s", group)
	err = removeOptionValue(c, section, "writable", project)
	if err != nil {
		return err
	}
	commitMsg := fmt.Sprintf("Removing project %s from group %s", project, group)
	return m.writeCommitPush(c, commitMsg)
}

// addGroup adds a new group to gitosis.conf. Also commit and push changes.
func (m *gitosisManager) addGroup(name string) error {
	c, err := m.getConfig()
	if err != nil {
		return err
	}
	sName := fmt.Sprintf("group %s", name)
	if !c.AddSection(sName) {
		errStr := fmt.Sprintf(`Could not add section "group %s" in gitosis.conf, section already exists!`, name)
		return errors.New(errStr)
	}
	commitMsg := fmt.Sprintf("Defining gitosis group for group %s", name)
	return m.writeCommitPush(c, commitMsg)
}

// removeGroup removes a group section and all it's options.
func (m *gitosisManager) removeGroup(group string) error {
	c, err := m.getConfig()
	if err != nil {
		return err
	}
	gName := fmt.Sprintf("group %s", group)
	if !c.RemoveSection(gName) {
		return errors.New("Section does not exists")
	}
	commitMsg := fmt.Sprintf("Removing group %s from gitosis.conf", group)
	return m.writeCommitPush(c, commitMsg)
}

// hasGroup checks if gitosis has the given group.
func (m *gitosisManager) hasGroup(group string) bool {
	c, err := m.getConfig()
	if err != nil {
		return false
	}
	return c.HasSection("group " + group)
}

// addMember adds a member to the given group.
//
// It is up to the caller make sure that the member does
// have a key in the keydir, otherwise the member will not
// be able to push to the repository.
func (m *gitosisManager) addMember(group, member string) error {
	c, err := m.getConfig()
	if err != nil {
		return err
	}
	section := fmt.Sprintf("group %s", group)
	if !c.HasSection(section) {
		return errors.New("Group not found")
	}
	if err = addOptionValue(c, section, "members", member); err != nil {
		return err
	}
	commitMsg := fmt.Sprintf("Adding member %s to group %s", member, group)
	return m.writeCommitPush(c, commitMsg)
}

// removeMember removes a member from the given group.
//
// It is up to the caller to delete the keyfile from the keydir
// using the DeleteKeyFile function.
func (m *gitosisManager) removeMember(group, member string) error {
	c, err := m.getConfig()
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
	return m.writeCommitPush(c, commitMsg)
}

// buildAndStoreKeyFile receives the key and writes it to disk, returning the
// name of the file written to disk and an error, if any happens.
func (m *gitosisManager) buildAndStoreKeyFile(member, key string) (string, error) {
	p := m.git.getPath("keydir")
	err := os.MkdirAll(p, 0755)
	if err != nil {
		return "", err
	}
	filename, err := nextAvailableKey(p, member)
	if err != nil {
		return "", err
	}
	keyfilename := path.Join(p, filename)
	keyfile, err := os.OpenFile(keyfilename, syscall.O_WRONLY|syscall.O_CREAT, 0644)
	if err != nil {
		return "", err
	}
	defer keyfile.Close()
	n, err := keyfile.WriteString(key)
	if err != nil || n != len(key) {
		return "", err
	}
	commitMsg := fmt.Sprintf("Added %s keyfile.", filename)
	err = m.commit(commitMsg)
	return filename, nil
}

// deleteKeyFile deletes the given key filename from the disk.
//
// It returns an error if the file does not exist or it is not possible to
// delete it.
func (m *gitosisManager) deleteKeyFile(keyfilename string) error {
	p := m.git.getPath("keydir")
	keypath := path.Join(p, keyfilename)
	err := os.Remove(keypath)
	if err != nil {
		return err
	}
	commitMsg := fmt.Sprintf("Deleted %s keyfile.", keyfilename)
	return m.commit(commitMsg)
}

// nextAvailableKey returns the next available key file for the given member.
//
// If the member is gopher@golang.org and there were three files (keys) for
// this member in the keydirname:
//
//   1. gopher@golang.org.key1.pub
//   2. gopher@golang.org.key2.pub
//   3. gopher@golang.org.key3.pub
//
// This function would return gopher@golang.org.key4.pub.
func nextAvailableKey(keydirname, member string) (string, error) {
	keydir, err := os.Open(keydirname)
	if err != nil {
		return "", err
	}
	defer keydir.Close()
	filenames, err := keydir.Readdirnames(0)
	if err != nil {
		return "", err
	}
	pattern := member + ".key%d.pub"
	counter := 1
	filename := fmt.Sprintf(pattern, counter)
	for _, f := range filenames {
		if f == filename {
			counter++
			filename = fmt.Sprintf(pattern, counter)
		}
	}
	return filename, nil
}

// getConfig reads config from gitosis.conf file.
func (m *gitosisManager) getConfig() (*ini.Config, error) {
	m.RLock()
	defer m.RUnlock()
	return ini.Read(m.confPath, ini.DEFAULT_COMMENT, ini.ALTERNATIVE_SEPARATOR, true, true)
}

// writeConfig writes the given config object to the gitosis.conf file.
func (m *gitosisManager) writeConfig(c *ini.Config) error {
	m.Lock()
	defer m.Unlock()
	return c.WriteFile(m.confPath, 0644, "gitosis config file")
}

// writeCommitPush write the given config object to the gitosis.conf file,
// commit and pushes.
func (m *gitosisManager) writeCommitPush(c *ini.Config, commitMsg string) error {
	err := m.writeConfig(c)
	if err != nil {
		return err
	}
	return m.commit(commitMsg)
}

// commit commits a change to the repository.
func (m *gitosisManager) commit(message string) error {
	err := m.git.commit(message)
	if err != nil {
		return err
	}
	return m.git.push("origin", "master")
}

// addOptionValue adds a value to an option.
//
// For example, if there is an option "favorite_maskots" in the ini file with
// the value "gopher", and we want to add "glenda" to this same option, the
// function will merge them and set "favorite_maskots=gopher glenda".
//
// If the option is not present in the file, it will be added.
func addOptionValue(c *ini.Config, section, option, value string) (err error) {
	var strValues string
	if c.HasOption(section, option) {
		strValues, err = c.String(section, option)
		if err != nil {
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

// removeOptionValue removes a value from an option.
//
// For example, if there is an option "good_os" in the ini file with the value
// "windows linux", and we want to remove "windows" from this option, the
// function will remove and keep "linux".
//
// If the option has only one value, it will be removed from the file.
func removeOptionValue(c *ini.Config, section, option, value string) (err error) {
	//check if section and option exists
	strValues, err := c.String(section, option)
	if err != nil {
		return err
	}
	values := strings.Split(strValues, " ")
	index := find(values, value)
	if index < 0 {
		return fmt.Errorf("Value %s not found in section %s", value, section)
	}
	copy(values[index:], values[index+1:])
	values = values[:len(values)-1]
	if len(values) > 0 {
		c.AddOption(section, option, strings.Join(values, " "))
	} else {
		c.RemoveOption(section, option)
	}

	return nil
}

// find looks for a string in a slice of strings, returning the index of the
// string, or -1 if the string is not present.
func find(strs []string, str string) int {
	for i, s := range strs {
		if str == s {
			return i
		}
	}
	return -1
}

// checkPresenceOfString checks if a given string is in a given slice of
// strings.
func checkPresenceOfString(strs []string, str string) bool {
	return find(strs, str) > -1
}
