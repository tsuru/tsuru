// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/globocom/config"
	"github.com/globocom/tsuru/db"
	"io"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"os"
	"path/filepath"
	"testing"
)

type hasKeyChecker struct{}

func (c *hasKeyChecker) Info() *CheckerInfo {
	return &CheckerInfo{Name: "HasKey", Params: []string{"user", "key"}}
}

func (c *hasKeyChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 2 {
		return false, "you should provide two parameters"
	}
	user, ok := params[0].(*User)
	if !ok {
		return false, "first parameter should be a user pointer"
	}
	content, ok := params[1].(string)
	if !ok {
		return false, "second parameter should be a string"
	}
	key := Key{Content: content}
	return user.hasKey(key), ""
}

var HasKey Checker = &hasKeyChecker{}

func Test(t *testing.T) { TestingT(t) }

type S struct {
	session *mgo.Session
	user    *User
	team    *Team
	token   *Token
	gitRoot string
}

var _ = Suite(&S{})

var panicIfErr = func(err error) {
	if err != nil {
		panic(err)
	}
}

func (s *S) SetUpSuite(c *C) {
	err := config.ReadConfigFile("../../etc/tsuru.conf")
	c.Assert(err, IsNil)
	db.Session, _ = db.Open("localhost:27017", "tsuru_user_test")
	s.user = &User{Email: "timeredbull@globo.com", Password: "123"}
	s.user.Create()
	s.token, _ = s.user.CreateToken()
	err = createTeam("cobrateam", s.user)
	panicIfErr(err)
	s.team = new(Team)
	err = db.Session.Teams().Find(bson.M{"_id": "cobrateam"}).One(s.team)
	panicIfErr(err)
}

func (s *S) TearDownSuite(c *C) {
	defer db.Session.Close()
	db.Session.Apps().Database.DropDatabase()
}

func (s *S) TearDownTest(c *C) {
	_, err := db.Session.Users().RemoveAll(bson.M{"email": bson.M{"$ne": s.user.Email}})
	panicIfErr(err)
	_, err = db.Session.Teams().RemoveAll(bson.M{"_id": bson.M{"$ne": s.team.Name}})
	panicIfErr(err)
}

func (s *S) getTestData(path ...string) io.ReadCloser {
	path = append([]string{}, ".", "testdata")
	p := filepath.Join(path...)
	f, _ := os.OpenFile(p, os.O_RDONLY, 0)
	return f
}
