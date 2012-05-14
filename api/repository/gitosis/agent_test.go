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

func (s *S) TestShouldHaveConstantForRemoveKey(c *C) {
	c.Assert(RemoveKey, Equals, 1)
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
	select {
	case k := <-response:
		c.Assert(k, Equals, "alanis-morissette_key1.pub")
	case <-time.After(1e9):
		c.Error("The AddKey change did not returned the key file name.")
	}
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

func (s *S) TestShouldHaveConstantForRemoveMember(c *C) {
	c.Assert(RemoveMember, Equals, 3)
}

func (s *S) TestAddMemberChangeAddsTheMemberToTheFile(c *C) {
	err := AddGroup("dream-theater")
	c.Assert(err, IsNil)
	change := Change{
		Kind: AddMember,
		Args: map[string]string{"group": "dream-theater", "member": "octavarium"},
	}
	Changes <- change
	time.Sleep(1e9)
	gitosis, err := os.Open(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	defer gitosis.Close()
	content, err := ioutil.ReadAll(gitosis)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(string(content), "members = octavarium"), Equals, true)
}

func (s *S) TestRemoveMemberChangeRemovesTheMemberFromTheFile(c *C) {
	err := AddGroup("dream-theater")
	c.Assert(err, IsNil)
	err = addMember("dream-theater", "the-glass-prision")
	c.Assert(err, IsNil)
	change := Change{
		Kind: RemoveMember,
		Args: map[string]string{"group": "dream-theater", "member": "the-glass-prision"},
	}
	Changes <- change
	time.Sleep(1e9)
	gitosis, err := os.Open(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	defer gitosis.Close()
	content, err := ioutil.ReadAll(gitosis)
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(string(content), "members = the-glass-prision"), Equals, false)
}
