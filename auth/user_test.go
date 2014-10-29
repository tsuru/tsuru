// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/testing"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gocheck"
)

func (s *S) TestCreateUser(c *gocheck.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	var result User
	collection := s.conn.Users()
	err = collection.Find(bson.M{"email": u.Email}).One(&result)
	c.Assert(err, gocheck.IsNil)
	c.Assert(result.Email, gocheck.Equals, u.Email)
}

func (s *S) TestCreateUserReturnsErrorWhenTryingToCreateAUserWithDuplicatedEmail(c *gocheck.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	err = u.Create()
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestGetUserByEmail(c *gocheck.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	u2, err := GetUserByEmail(u.Email)
	c.Assert(err, gocheck.IsNil)
	c.Check(u2.Email, gocheck.Equals, u.Email)
	c.Check(u2.Password, gocheck.Equals, u.Password)
}

func (s *S) TestGetUserByEmailReturnsErrorWhenNoUserIsFound(c *gocheck.C) {
	u, err := GetUserByEmail("unknown@globo.com")
	c.Assert(u, gocheck.IsNil)
	c.Assert(err, gocheck.Equals, ErrUserNotFound)
}

func (s *S) TestGetUserByEmailWithInvalidEmail(c *gocheck.C) {
	u, err := GetUserByEmail("unknown")
	c.Assert(u, gocheck.IsNil)
	c.Assert(err, gocheck.NotNil)
	e, ok := err.(*errors.ValidationError)
	c.Assert(ok, gocheck.Equals, true)
	c.Assert(e.Message, gocheck.Equals, "Invalid email.")
}

func (s *S) TestUpdateUser(c *gocheck.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	u.Password = "1234"
	err = u.Update()
	c.Assert(err, gocheck.IsNil)
	u2, err := GetUserByEmail("wolverine@xmen.com")
	c.Assert(err, gocheck.IsNil)
	c.Assert(u2.Password, gocheck.Equals, "1234")
}

func (s *S) TestAddKeyAddsAKeyToTheUser(c *gocheck.C) {
	var request *http.Request
	var content []byte
	server := testing.StartGandalfTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		request = r
		content, _ = ioutil.ReadAll(r.Body)
	}))
	defer server.Close()
	u := &User{Email: "sacefulofsecrets@pinkfloyd.com"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer u.Delete()
	key := Key{Name: "some-key", Content: "my-key"}
	err = u.AddKey(key)
	c.Assert(err, gocheck.IsNil)
	c.Assert(u, HasKey, "my-key")
	expectedPath := fmt.Sprintf("/user/%s/key", u.Email)
	expectedBody := `{"sacefulofsecrets@pinkfloyd.com-1":"my-key"}`
	c.Assert(request.Method, gocheck.Equals, "POST")
	c.Assert(request.URL.Path, gocheck.Equals, expectedPath)
	c.Assert(string(content), gocheck.Equals, expectedBody)
}

func (s *S) TestRemoveKeyRemovesAKeyFromTheUser(c *gocheck.C) {
	var request *http.Request
	server := testing.StartGandalfTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r
	}))
	defer server.Close()
	key := Key{Content: "my-key", Name: "the-key"}
	u := &User{Email: "shineon@pinkfloyd.com", Keys: []Key{key}}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	err = u.RemoveKey(Key{Content: "my-key"})
	c.Assert(err, gocheck.IsNil)
	c.Assert(u, gocheck.Not(HasKey), "my-key")
	expectedPath := fmt.Sprintf("/user/%s/key/%s", u.Email, key.Name)
	c.Assert(request.Method, gocheck.Equals, "DELETE")
	c.Assert(request.URL.Path, gocheck.Equals, expectedPath)
}

func (s *S) TestRemoveUnknownKey(c *gocheck.C) {
	u := &User{Email: "shine@pinkfloyd.com", Keys: nil}
	err := u.RemoveKey(Key{Content: "my-key"})
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, "key not found")
}

func (s *S) TestTeams(c *gocheck.C) {
	u := User{Email: "me@tsuru.com", Password: "123"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	s.team.AddUser(&u)
	err = s.conn.Teams().Update(bson.M{"_id": s.team.Name}, s.team)
	c.Assert(err, gocheck.IsNil)
	defer func(u *User, t *Team) {
		t.RemoveUser(u)
		s.conn.Teams().Update(bson.M{"_id": t.Name}, t)
	}(&u, s.team)
	t := Team{Name: "abc", Users: []string{u.Email}}
	err = s.conn.Teams().Insert(t)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": t.Name})
	teams, err := u.Teams()
	c.Assert(err, gocheck.IsNil)
	c.Assert(teams, gocheck.HasLen, 2)
	c.Assert(teams[0].Name, gocheck.Equals, s.team.Name)
	c.Assert(teams[1].Name, gocheck.Equals, t.Name)
}

func (s *S) TestFindKeyReturnsKeyWithNameAndContent(c *gocheck.C) {
	u := User{Email: "me@tsuru.com", Password: "123", Keys: []Key{{Name: "me@tsuru.com-1", Content: "ssh-rsa somekey me@tsuru.com"}}}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	k, index := u.FindKey(Key{Content: u.Keys[0].Content})
	c.Assert(index, gocheck.Equals, 0)
	c.Assert(k.Name, gocheck.Equals, u.Keys[0].Name)
}

func (s *S) TestIsAdminReturnsTrueWhenUserHasATeamNamedWithAdminTeamConf(c *gocheck.C) {
	adminTeamName, err := config.GetString("admin-team")
	c.Assert(err, gocheck.IsNil)
	t := Team{Name: adminTeamName, Users: []string{s.user.Email}}
	err = s.conn.Teams().Insert(&t)
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Teams().RemoveId(t.Name)
	c.Assert(s.user.IsAdmin(), gocheck.Equals, true)
}

func (s *S) TestIsAdminReturnsFalseWhenUserDoNotHaveATeamNamedWithAdminTeamConf(c *gocheck.C) {
	c.Assert(s.user.IsAdmin(), gocheck.Equals, false)
}

type testApp struct {
	Name  string
	Teams []string
}

func (s *S) TestUserAllowedApps(c *gocheck.C) {
	team := Team{Name: "teamname", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(&team)
	c.Assert(err, gocheck.IsNil)
	a := testApp{Name: "myapp", Teams: []string{s.team.Name}}
	err = s.conn.Apps().Insert(&a)
	c.Assert(err, gocheck.IsNil)
	a2 := testApp{Name: "myotherapp", Teams: []string{team.Name}}
	err = s.conn.Apps().Insert(&a2)
	c.Assert(err, gocheck.IsNil)
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": bson.M{"$in": []string{a.Name, a2.Name}}})
		s.conn.Teams().RemoveId(team.Name)
	}()
	aApps, err := s.user.AllowedApps()
	c.Assert(aApps, gocheck.DeepEquals, []string{a.Name, a2.Name})
}

func (s *S) TestListKeysShouldCallGandalfAPI(c *gocheck.C) {
	h := testHandler{content: `{"mypckey":"ssh-rsa keystuff keycomment"}`}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	keys, err := u.ListKeys()
	c.Assert(err, gocheck.IsNil)
	expected := map[string]string{"mypckey": "ssh-rsa keystuff keycomment"}
	c.Assert(expected, gocheck.DeepEquals, keys)
	c.Assert(h.url[0], gocheck.Equals, "/user/wolverine@xmen.com/keys")
	c.Assert(h.method[0], gocheck.Equals, "GET")
}

func (s *S) TestListKeysGandalfAPIError(c *gocheck.C) {
	h := testBadHandler{content: "some terrible error"}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	keys, err := u.ListKeys()
	c.Assert(keys, gocheck.DeepEquals, map[string]string(nil))
	c.Assert(err.Error(), gocheck.Equals, "some terrible error\n")
}

func (s *S) TestKeyToMap(c *gocheck.C) {
	keys := []Key{{Name: "testkey", Content: "somekey"}}
	keysMap := keyToMap(keys)
	c.Assert(keysMap, gocheck.DeepEquals, map[string]string{"testkey": "somekey"})
}

func (s *S) TestAddKeyInGandalfShouldCallGandalfAPI(c *gocheck.C) {
	h := testing.TestHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	u := &User{Email: "me@gmail.com"}
	err := u.Create()
	c.Assert(err, gocheck.IsNil)
	defer u.Delete()
	key := Key{Content: "my-ssh-key", Name: "key1"}
	err = u.addKeyGandalf(&key)
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.Url, gocheck.Equals, "/user/me@gmail.com/key")
}

func (s *S) TestCreateUserOnGandalf(c *gocheck.C) {
	h := testing.TestHandler{}
	ts := testing.StartGandalfTestServer(&h)
	defer ts.Close()
	u := &User{Email: "me@gmail.com"}
	err := u.CreateOnGandalf()
	c.Assert(err, gocheck.IsNil)
	c.Assert(h.Url, gocheck.Equals, "/user")
	expected := `{"name":"me@gmail.com","keys":{}}`
	c.Assert(string(h.Body), gocheck.Equals, expected)
	c.Assert(h.Method, gocheck.Equals, "POST")
}
