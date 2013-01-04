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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	hashed  string
	user    *User
	team    *Team
	token   *Token
	gitRoot string
	gitHost string
	gitPort string
	gitProt string
}

var _ = Suite(&S{})

var panicIfErr = func(err error) {
	if err != nil {
		panic(err)
	}
}

func (s *S) SetUpSuite(c *C) {
	s.hashed = hashPassword("123")
	err := config.ReadConfigFile("../etc/tsuru.conf")
	c.Assert(err, IsNil)
	db.Session, _ = db.Open("localhost:27017", "tsuru_user_test")
	s.user = &User{Email: "timeredbull@globo.com", Password: "123"}
	s.user.Create()
	s.token, _ = s.user.CreateToken()
	team := &Team{Name: "cobrateam", Users: []string{s.user.Email}}
	err = db.Session.Teams().Insert(team)
	panicIfErr(err)
	s.team = team
	s.gitHost, _ = config.GetString("git:host")
	s.gitPort, _ = config.GetString("git:port")
	s.gitProt, _ = config.GetString("git:protocol")
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
	if s.user.Password != s.hashed {
		s.user.Password = s.hashed
		err = s.user.update()
		panicIfErr(err)
	}
	config.Set("git:host", s.gitHost)
	config.Set("git:port", s.gitPort)
	config.Set("git:protocol", s.gitProt)
}

func (s *S) getTestData(path ...string) io.ReadCloser {
	path = append([]string{}, ".", "testdata")
	p := filepath.Join(path...)
	f, _ := os.OpenFile(p, os.O_RDONLY, 0)
	return f
}

// starts a new httptest.Server and returns it
// Also changes git:host, git:port and git:protocol to match the server's url
func (s *S) startGandalfTestServer(h http.Handler) *httptest.Server {
	ts := httptest.NewServer(h)
	pieces := strings.Split(ts.URL, "://")
	protocol := pieces[0]
	hostPart := strings.Split(pieces[1], ":")
	port := hostPart[1]
	host := hostPart[0]
	config.Set("git:host", host)
	portInt, _ := strconv.ParseInt(port, 10, 0)
	config.Set("git:port", portInt)
	config.Set("git:protocol", protocol)
	return ts
}
