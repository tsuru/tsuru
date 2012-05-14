package gitosis

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"path"
	"syscall"
)

func (s *S) TestBuildAndStoreKeyFileAddsAKeyFileToTheKeydirDirectoryAndTheMemberToTheGroupAndReturnTheKeyFileName(c *C) {
	keyFileName, err := buildAndStoreKeyFile("tolices", "my-key")
	c.Assert(err, IsNil)
	c.Assert(keyFileName, Equals, "tolices_key1.pub")
	p, err := getKeydirPath()
	c.Assert(err, IsNil)
	filePath := path.Join(p, keyFileName)
	file, err := os.Open(filePath)
	c.Assert(err, IsNil)
	defer file.Close()
	content, err := ioutil.ReadAll(file)
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, "my-key")
}

func (s *S) TestBuildAndStoreKeyFileUseKey2IfThereIsAlreadyAKeyForTheMember(c *C) {
	p, err := getKeydirPath()
	c.Assert(err, IsNil)
	key1Path := path.Join(p, "gol-de-quem_key1.pub")
	f, err := os.OpenFile(key1Path, syscall.O_CREAT, 0644)
	c.Assert(err, IsNil)
	f.Close()
	keyFileName, err := buildAndStoreKeyFile("gol-de-quem", "my-key")
	c.Assert(err, IsNil)
	c.Assert(keyFileName, Equals, "gol-de-quem_key2.pub")
	file, err := os.Open(path.Join(p, keyFileName))
	c.Assert(err, IsNil)
	defer file.Close()
	content, err := ioutil.ReadAll(file)
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, "my-key")
}

func (s *S) TestBuildAndStoreKeyFileDoesNotReturnErrorIfTheDirectoryExists(c *C) {
	p, err := getKeydirPath()
	c.Assert(err, IsNil)
	os.MkdirAll(p, 0755)
	_, err = buildAndStoreKeyFile("vida-imbecil", "my-key")
	c.Assert(err, IsNil)
}

func (s *S) TestBuildAndStoreKeyFileCommits(c *C) {
	keyfile, err := buildAndStoreKeyFile("the-night-and-the-silent-water", "my-key")
	c.Assert(err, IsNil)
	got := s.lastBareCommit(c)
	expected := fmt.Sprintf("Added %s keyfile.", keyfile)
	c.Assert(got, Equals, expected)
}

func (s *S) TesteDeleteKeyFile(c *C) {
	keyfile, err := buildAndStoreKeyFile("blackwater-park", "my-key")
	c.Assert(err, IsNil)
	err = deleteKeyFile(keyfile)
	c.Assert(err, IsNil)
	p, err := getKeydirPath()
	c.Assert(err, IsNil)
	keypath := path.Join(p, keyfile)
	_, err = os.Stat(keypath)
	c.Assert(err, NotNil)
	c.Assert(os.IsNotExist(err), Equals, true)
}

func (s *S) TesteDeleteKeyFileReturnsErrorIfTheFileDoesNotExist(c *C) {
	err := deleteKeyFile("dont_know.pub")
	c.Assert(err, NotNil)
}

func (s *S) TesteDeleteKeyFileCommits(c *C) {
	keyfile, err := buildAndStoreKeyFile("windowpane", "my-key")
	c.Assert(err, IsNil)
	err = deleteKeyFile(keyfile)
	c.Assert(err, IsNil)
	expected := fmt.Sprintf("Deleted %s keyfile.", keyfile)
	got := s.lastBareCommit(c)
	c.Assert(got, Equals, expected)
}
