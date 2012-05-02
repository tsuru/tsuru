package auth

import (
	"github.com/timeredbull/tsuru/db"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo"
	"launchpad.net/mgo/bson"
	"os"
	"path/filepath"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type S struct {
	session *mgo.Session
	user    *User
	team    *Team
	token   *Token
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	db.Session, _ = db.Open("localhost:27017", "tsuru_user_test")
	s.user = &User{Email: "timeredbull@globo.com", Password: "123"}
	s.user.Create()
	s.token, _ = s.user.CreateToken()
	s.team = &Team{Name: "cobrateam", Users: []*User{s.user}}
	db.Session.Teams().Insert(s.team)
}

func (s *S) TearDownSuite(c *C) {
	defer db.Session.Close()
	db.Session.DropDB()
}

func (s *S) TearDownTest(c *C) {
	err := db.Session.Users().RemoveAll(bson.M{"email": bson.M{"$ne": s.user.Email}})
	c.Assert(err, IsNil)
	err = db.Session.Teams().RemoveAll(bson.M{"name": bson.M{"$ne": s.team.Name}})
	c.Assert(err, IsNil)
}

func (s *S) getTestData(path ...string) io.ReadCloser {
	path = append([]string{}, ".", "testdata")
	p := filepath.Join(path...)
	f, _ := os.OpenFile(p, os.O_RDONLY, 0)
	return f
}
