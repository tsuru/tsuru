package gitosis

import (
	ini "github.com/kless/goconfig/config"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
	"path"
	"syscall"
)

func (s *S) TestAddKeyAddsAKeyFileToTheKeydirDirectoryAndTheMemberToTheGroup(c *C) {
	err := AddGroup("pato-fu")
	c.Assert(err, IsNil)
	err = AddKey("pato-fu", "tolices", "my-key")
	c.Assert(err, IsNil)
	p, err := getKeydirPath()
	c.Assert(err, IsNil)
	filePath := path.Join(p, "tolices_key1.pub")
	file, err := os.Open(filePath)
	c.Assert(err, IsNil)
	defer file.Close()
	content, err := ioutil.ReadAll(file)
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, "my-key")
	conf, err := ini.ReadDefault(path.Join(s.gitosisRepo, "gitosis.conf"))
	c.Assert(err, IsNil)
	members, err := conf.String("group pato-fu", "members")
	c.Assert(err, IsNil)
	c.Assert(members, Equals, "tolices_key1")
}

func (s *S) TestAddKeyUseKey2IfThereIsAlreadyAKeyForTheMember(c *C) {
	err := AddGroup("pato-fu")
	c.Assert(err, IsNil)
	p, err := getKeydirPath()
	c.Assert(err, IsNil)
	key1Path := path.Join(p, "gol-de-quem_key1.pub")
	f, err := os.OpenFile(key1Path, syscall.O_CREAT, 0644)
	c.Assert(err, IsNil)
	f.Close()
	err = AddKey("pato-fu", "gol-de-quem", "my-key")
	c.Assert(err, IsNil)
	file, err := os.Open(path.Join(p, "gol-de-quem_key2.pub"))
	c.Assert(err, IsNil)
	defer file.Close()
	content, err := ioutil.ReadAll(file)
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, "my-key")
}

func (s *S) TestAddKeyReturnsErrorIfTheGroupDoesNotExist(c *C) {
	err := AddKey("pato-fu", "sertoes", "my-key")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Group not found$")
}

func (s *S) TestAddKeyDoesNotReturnErrorIfTheDirectoryExists(c *C) {
	err := AddGroup("pato-fu")
	c.Assert(err, IsNil)
	p, err := getKeydirPath()
	c.Assert(err, IsNil)
	os.MkdirAll(p, 0755)
	err = AddKey("pato-fu", "vida-imbecil", "my-key")
	c.Assert(err, IsNil)
}

func (s *S) TestAddKeyShouldRemoveTheKeyFileIfItFailsToAddTheMemberToGitosisFile(c *C) {
	err := AddGroup("pain-of-salvation")
	c.Assert(err, IsNil)
	err = addMember("pain-of-salvation", "used_key1")
	c.Assert(err, IsNil)
	err = AddKey("pain-of-salvation", "used", "my-key")
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Failed to add member to the group, the key file was not saved$")
	p, err := getKeydirPath()
	c.Assert(err, IsNil)
	filepath := path.Join(p, "used_key1.pub")
	f, err := os.Open(filepath)
	if f != nil {
		defer f.Close()
	}
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true)
}

func (s *S) TestAddKeyShouldCommit(c *C) {
	err := AddGroup("pain-of-salvation")
	c.Assert(err, IsNil)
	err = AddKey("pain-of-salvation", "diffidentia", "my-key")
	c.Assert(err, IsNil)
	pwd, err := os.Getwd()
	c.Assert(err, IsNil)
	os.Chdir(s.gitosisBare)
	bareOutput, err := exec.Command("git", "log", "-1", "--pretty=format:%s").CombinedOutput()
	c.Assert(err, IsNil)
	os.Chdir(pwd)
	commitMsg := "Adding member diffidentia_key1 to group pain-of-salvation"
	c.Assert(string(bareOutput), Equals, commitMsg)
}
