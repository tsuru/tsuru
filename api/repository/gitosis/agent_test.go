package gitosis

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"path"
	"strings"
	"time"
)

func (s *S) TestShouldHaveConstantForAddKey(c *C) {
	c.Assert(AddKey, Equals, 0)
}

func (s *S) TestAddKeyReturnsTheKeyFileNameInTheResponseChannel(c *C) {
	response := make(chan string)
	change := Change{
		Kind: AddKey,
		Args: map[string]string{
			"key":    "so-pure",
			"member": "alanis-morissette",
		},
		Response: response,
	}
	Changes <- change
	k := <-response
	c.Assert(k, Equals, "alanis-morissette_key1.pub")
}

func (s *S) TestShouldHaveConstantForRemoveKey(c *C) {
	c.Assert(RemoveKey, Equals, 1)
}

func (s *S) TestRemoveKeyChangeRemovesTheKey(c *C) {
	keyfile, err := buildAndStoreKeyFile("alanis-morissette", "your-house")
	c.Assert(err, IsNil)
	change := Change{
		Kind: RemoveKey,
		Args: map[string]string{"key": keyfile},
	}
	Changes <- change
	time.Sleep(1e9)
	p, err := getKeydirPath()
	c.Assert(err, IsNil)
	keypath := path.Join(p, keyfile)
	_, err = os.Stat(keypath)
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true)
}

func (s *S) TestShouldHaveConstantForAddMember(c *C) {
	c.Assert(AddMember, Equals, 2)
}

func (s *S) TestAddMemberChangeAddsTheMemberToTheFile(c *C) {
	err := addGroup("dream-theater")
	c.Assert(err, IsNil)
	change := Change{
		Kind:     AddMember,
		Args:     map[string]string{"group": "dream-theater", "member": "octavarium"},
		Response: make(chan string),
	}
	Changes <- change
	<-change.Response
	gitosis, err := os.Open(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	defer gitosis.Close()
	content, err := ioutil.ReadAll(gitosis)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(string(content), "members = octavarium"), Equals, true)
}

func (s *S) TestShouldHaveConstantForRemoveMember(c *C) {
	c.Assert(RemoveMember, Equals, 3)
}

func (s *S) TestRemoveMemberChangeRemovesTheMemberFromTheFile(c *C) {
	err := addGroup("dream-theater")
	c.Assert(err, IsNil)
	err = addMember("dream-theater", "the-glass-prision")
	c.Assert(err, IsNil)
	change := Change{
		Kind:     RemoveMember,
		Args:     map[string]string{"group": "dream-theater", "member": "the-glass-prision"},
		Response: make(chan string),
	}
	Changes <- change
	<-change.Response
	gitosis, err := os.Open(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	defer gitosis.Close()
	content, err := ioutil.ReadAll(gitosis)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(string(content), "members = the-glass-prision"), Equals, false)
}

func (s *S) TestShouldHaveConstantForAddGroup(c *C) {
	c.Assert(AddGroup, Equals, 4)
}

func (s *S) TestAddGroupChangeAddsAGroupToGitosisConf(c *C) {
	change := Change{
		Kind:     AddGroup,
		Args:     map[string]string{"group": "dream-theater"},
		Response: make(chan string),
	}
	Changes <- change
	<-change.Response
	gitosis, err := os.Open(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	defer gitosis.Close()
	content, err := ioutil.ReadAll(gitosis)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(string(content), "[group dream-theater]"), Equals, true)
}

func (s *S) TestShouldHaveConstantForRemoveGroup(c *C) {
	c.Assert(RemoveGroup, Equals, 5)
}

func (s *S) TestRemoveGroupChangeRemovesTheGroupFromGitosisConf(c *C) {
	err := addGroup("steve-lee")
	c.Assert(err, IsNil)
	change := Change{
		Kind:     RemoveGroup,
		Args:     map[string]string{"group": "steve-lee"},
		Response: make(chan string),
	}
	Changes <- change
	<-change.Response
	gitosis, err := os.Open(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	defer gitosis.Close()
	content, err := ioutil.ReadAll(gitosis)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(string(content), "[group steve-lee]"), Equals, false)
}

func (s *S) TestShouldHaveConstantForAddProject(c *C) {
	c.Assert(AddProject, Equals, 6)
}

func (s *S) TestAddProjectChangeAddsAProjectToTheGroup(c *C) {
	err := addGroup("rush")
	c.Assert(err, IsNil)
	change := Change{
		Kind:     AddProject,
		Args:     map[string]string{"group": "rush", "project": "grace-under-pressure"},
		Response: make(chan string),
	}
	Changes <- change
	<-change.Response
	gitosis, err := os.Open(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	defer gitosis.Close()
	content, err := ioutil.ReadAll(gitosis)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(string(content), "writable = grace-under-pressure"), Equals, true)
}

func (s *S) TestShouldHaveContantForRemoveProject(c *C) {
	c.Assert(RemoveProject, Equals, 7)
}

func (s *S) TestRemoveProjectChangeRemovesAProjectFromTheGroup(c *C) {
	err := addGroup("nando-reis")
	c.Assert(err, IsNil)
	err = addProject("nando-reis", "ao-vivo")
	c.Assert(err, IsNil)
	change := Change{
		Kind:     RemoveProject,
		Args:     map[string]string{"group": "nando-reis", "project": "ao-vivo"},
		Response: make(chan string),
	}
	Changes <- change
	<-change.Response
	gitosis, err := os.Open(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	defer gitosis.Close()
	content, err := ioutil.ReadAll(gitosis)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(string(content), "writable = ao-vivo"), Equals, false)
}
