package user

import (
	. "github.com/timeredbull/tsuru/database"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo"
	"os"
	"path/filepath"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	session *mgo.Session
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	var err error
	s.session, err = mgo.Dial("localhost:27017")
	c.Assert(err, IsNil)
	Mdb = s.session.DB("tsuru_test")
}

func (s *S) TearDownSuite(c *C) {
	s.session.Close()
}

func (s *S) TearDownTest(c *C) {
	err := Mdb.C("users").DropCollection()
	c.Assert(err, IsNil)
}

func (s *S) getTestData(path ...string) io.ReadCloser {
	path = append([]string{}, ".", "testdata")
	p := filepath.Join(path...)
	f, _ := os.OpenFile(p, os.O_RDONLY, 0)
	return f
}
