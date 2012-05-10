package gitosis

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"os"
	"path"
	"syscall"
)

func (s *S) TestBuildAndStoreKeyFileAddsAKeyFileToTheKeydirDirectoryAndTheMemberToTheGroupAndReturnTheKeyFileName(c *C) {
	keyFileName, err := BuildAndStoreKeyFile("tolices", "my-key")
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
	keyFileName, err := BuildAndStoreKeyFile("gol-de-quem", "my-key")
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
	_, err = BuildAndStoreKeyFile("vida-imbecil", "my-key")
	c.Assert(err, IsNil)
}
