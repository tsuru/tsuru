package gitosis

import (
	. "launchpad.net/gocheck"
	"os"
	"path"
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
