// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/apitest"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestCreateUser(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	var result User
	collection := s.conn.Users()
	err = collection.Find(bson.M{"email": u.Email}).One(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Email, check.Equals, u.Email)
}

func (s *S) TestCreateUserReturnsErrorWhenTryingToCreateAUserWithDuplicatedEmail(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	err = u.Create()
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetUserByEmail(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	u2, err := GetUserByEmail(u.Email)
	c.Assert(err, check.IsNil)
	c.Check(u2.Email, check.Equals, u.Email)
	c.Check(u2.Password, check.Equals, u.Password)
}

func (s *S) TestGetUserByEmailReturnsErrorWhenNoUserIsFound(c *check.C) {
	u, err := GetUserByEmail("unknown@globo.com")
	c.Assert(u, check.IsNil)
	c.Assert(err, check.Equals, ErrUserNotFound)
}

func (s *S) TestGetUserByEmailWithInvalidEmail(c *check.C) {
	u, err := GetUserByEmail("unknown")
	c.Assert(u, check.IsNil)
	c.Assert(err, check.NotNil)
	e, ok := err.(*errors.ValidationError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Message, check.Equals, "invalid email")
}

func (s *S) TestUpdateUser(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	u.Password = "1234"
	err = u.Update()
	c.Assert(err, check.IsNil)
	u2, err := GetUserByEmail("wolverine@xmen.com")
	c.Assert(err, check.IsNil)
	c.Assert(u2.Password, check.Equals, "1234")
}

func (s *S) TestAddKeyAddsAKeyToTheUser(c *check.C) {
	var request *http.Request
	var content []byte
	server := repositorytest.StartGandalfTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		request = r
		content, _ = ioutil.ReadAll(r.Body)
	}))
	defer server.Close()
	u := &User{Email: "sacefulofsecrets@pinkfloyd.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	key := Key{Name: "some-key", Content: "my-key"}
	err = u.AddKey(key)
	c.Assert(err, check.IsNil)
	c.Assert(u, HasKey, "my-key")
	expectedPath := fmt.Sprintf("/user/%s/key", u.Email)
	expectedBody := `{"some-key":"my-key"}`
	c.Assert(request.Method, check.Equals, "POST")
	c.Assert(request.URL.Path, check.Equals, expectedPath)
	c.Assert(string(content), check.Equals, expectedBody)
}

func (s *S) TestAddKeyGeneratesNameWhenEmpty(c *check.C) {
	var request *http.Request
	var content []byte
	server := repositorytest.StartGandalfTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		request = r
		content, _ = ioutil.ReadAll(r.Body)
	}))
	defer server.Close()
	u := &User{Email: "sacefulofsecrets@pinkfloyd.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	key := Key{Content: "my-key"}
	err = u.AddKey(key)
	c.Assert(err, check.IsNil)
	c.Assert(u, HasKey, "my-key")
	expectedPath := fmt.Sprintf("/user/%s/key", u.Email)
	expectedBody := `{"sacefulofsecrets@pinkfloyd.com-1":"my-key"}`
	c.Assert(request.Method, check.Equals, "POST")
	c.Assert(request.URL.Path, check.Equals, expectedPath)
	c.Assert(string(content), check.Equals, expectedBody)
}

func (s *S) TestAddDuplicatedKey(c *check.C) {
	u := &User{
		Email: "sacefulofsecrets@pinkfloyd.com",
		Keys:  []Key{{Name: "my-key", Content: "some-key"}},
	}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	key := Key{Name: "my-key", Content: "other-key"}
	err = u.AddKey(key)
	c.Assert(err, check.Equals, ErrUserAlreadyHasKey)
}

func (s *S) TestRemoveKeyRemovesAKeyFromTheUser(c *check.C) {
	var request *http.Request
	server := repositorytest.StartGandalfTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r
	}))
	defer server.Close()
	key := Key{Content: "my-key", Name: "the-key"}
	u := &User{Email: "shineon@pinkfloyd.com", Keys: []Key{key}}
	err := u.Create()
	c.Assert(err, check.IsNil)
	err = u.RemoveKey(Key{Content: "my-key"})
	c.Assert(err, check.IsNil)
	c.Assert(u, check.Not(HasKey), "my-key")
	expectedPath := fmt.Sprintf("/user/%s/key/%s", u.Email, key.Name)
	c.Assert(request.Method, check.Equals, "DELETE")
	c.Assert(request.URL.Path, check.Equals, expectedPath)
}

func (s *S) TestRemoveUnknownKey(c *check.C) {
	u := &User{Email: "shine@pinkfloyd.com", Keys: nil}
	err := u.RemoveKey(Key{Content: "my-key"})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "key not found")
}

func (s *S) TestTeams(c *check.C) {
	u := User{Email: "me@tsuru.com", Password: "123"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	s.team.AddUser(&u)
	err = s.conn.Teams().Update(bson.M{"_id": s.team.Name}, s.team)
	c.Assert(err, check.IsNil)
	defer func(u *User, t *Team) {
		t.RemoveUser(u)
		s.conn.Teams().Update(bson.M{"_id": t.Name}, t)
	}(&u, s.team)
	t := Team{Name: "abc", Users: []string{u.Email}}
	err = s.conn.Teams().Insert(t)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().Remove(bson.M{"_id": t.Name})
	teams, err := u.Teams()
	c.Assert(err, check.IsNil)
	c.Assert(teams, check.HasLen, 2)
	c.Assert(teams[0].Name, check.Equals, s.team.Name)
	c.Assert(teams[1].Name, check.Equals, t.Name)
}

func (s *S) TestFindKeyByName(c *check.C) {
	u := User{
		Email:    "me@tsuru.com",
		Password: "123",
		Keys:     []Key{{Name: "me@tsuru.com-1", Content: "ssh-rsa somekey me@tsuru.com"}},
	}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	k, index := u.FindKey(Key{Name: u.Keys[0].Name})
	c.Assert(index, check.Equals, 0)
	c.Assert(k.Name, check.Equals, u.Keys[0].Name)
}

func (s *S) TestFindKeyByBody(c *check.C) {
	u := User{
		Email:    "me@tsuru.com",
		Password: "123",
		Keys:     []Key{{Name: "me@tsuru.com-1", Content: "ssh-rsa somekey me@tsuru.com"}},
	}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	k, index := u.FindKey(Key{Content: u.Keys[0].Content})
	c.Assert(index, check.Equals, 0)
	c.Assert(k.Name, check.Equals, u.Keys[0].Name)
}

func (s *S) TestIsAdminReturnsTrueWhenUserHasATeamNamedWithAdminTeamConf(c *check.C) {
	adminTeamName, err := config.GetString("admin-team")
	c.Assert(err, check.IsNil)
	t := Team{Name: adminTeamName, Users: []string{s.user.Email}}
	err = s.conn.Teams().Insert(&t)
	c.Assert(err, check.IsNil)
	defer s.conn.Teams().RemoveId(t.Name)
	c.Assert(s.user.IsAdmin(), check.Equals, true)
}

func (s *S) TestIsAdminReturnsFalseWhenUserDoNotHaveATeamNamedWithAdminTeamConf(c *check.C) {
	c.Assert(s.user.IsAdmin(), check.Equals, false)
}

type testApp struct {
	Name  string
	Teams []string
}

func (s *S) TestUserAllowedApps(c *check.C) {
	team := Team{Name: "teamname", Users: []string{s.user.Email}}
	err := s.conn.Teams().Insert(&team)
	c.Assert(err, check.IsNil)
	a := testApp{Name: "myapp", Teams: []string{s.team.Name}}
	err = s.conn.Apps().Insert(&a)
	c.Assert(err, check.IsNil)
	a2 := testApp{Name: "myotherapp", Teams: []string{team.Name}}
	err = s.conn.Apps().Insert(&a2)
	c.Assert(err, check.IsNil)
	defer func() {
		s.conn.Apps().Remove(bson.M{"name": bson.M{"$in": []string{a.Name, a2.Name}}})
		s.conn.Teams().RemoveId(team.Name)
	}()
	aApps, err := s.user.AllowedApps()
	c.Assert(aApps, check.DeepEquals, []string{a.Name, a2.Name})
}

func (s *S) TestListKeysShouldCallGandalfAPI(c *check.C) {
	h := testHandler{content: `{"mypckey":"ssh-rsa keystuff keycomment"}`}
	ts := repositorytest.StartGandalfTestServer(&h)
	defer ts.Close()
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	keys, err := u.ListKeys()
	c.Assert(err, check.IsNil)
	expected := map[string]string{"mypckey": "ssh-rsa keystuff keycomment"}
	c.Assert(expected, check.DeepEquals, keys)
	c.Assert(h.url[0], check.Equals, "/user/wolverine@xmen.com/keys")
	c.Assert(h.method[0], check.Equals, "GET")
}

func (s *S) TestListKeysGandalfAPIError(c *check.C) {
	h := testBadHandler{content: "some terrible error"}
	ts := repositorytest.StartGandalfTestServer(&h)
	defer ts.Close()
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	keys, err := u.ListKeys()
	c.Assert(keys, check.DeepEquals, map[string]string(nil))
	c.Assert(err.Error(), check.Equals, "some terrible error\n")
}

func (s *S) TestKeyToMap(c *check.C) {
	keys := []Key{{Name: "testkey", Content: "somekey"}}
	keysMap := keyToMap(keys)
	c.Assert(keysMap, check.DeepEquals, map[string]string{"testkey": "somekey"})
}

func (s *S) TestAddKeyInGandalfShouldCallGandalfAPI(c *check.C) {
	h := apitest.TestHandler{}
	ts := repositorytest.StartGandalfTestServer(&h)
	defer ts.Close()
	u := &User{Email: "me@gmail.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	key := Key{Content: "my-ssh-key", Name: "key1"}
	err = u.addKeyGandalf(&key)
	c.Assert(err, check.IsNil)
	c.Assert(h.Url, check.Equals, "/user/me@gmail.com/key")
}

func (s *S) TestCreateUserOnGandalf(c *check.C) {
	h := apitest.TestHandler{}
	ts := repositorytest.StartGandalfTestServer(&h)
	defer ts.Close()
	u := &User{Email: "me@gmail.com"}
	err := u.CreateOnGandalf()
	c.Assert(err, check.IsNil)
	c.Assert(h.Url, check.Equals, "/user")
	expected := `{"name":"me@gmail.com","keys":{}}`
	c.Assert(string(h.Body), check.Equals, expected)
	c.Assert(h.Method, check.Equals, "POST")
}

func (s *S) TestShowAPIKeyWhenAPITokenAlreadyExists(c *check.C) {
	u := User{
		Email:    "me@tsuru.com",
		Password: "123",
		Keys:     []Key{{Name: "me@tsuru.com-1", Content: "ssh-rsa somekey me@tsuru.com"}},
		APIKey:   "1ioudh8ydb2idn1ehnpoqwjmhdjqwz12po1",
	}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	API_Token, err := u.ShowAPIKey()
	c.Assert(API_Token, check.Equals, u.APIKey)
	c.Assert(err, check.IsNil)
}

func (s *S) TestShowAPIKeyWhenAPITokenNotExists(c *check.C) {
	u := User{
		Email:    "me@tsuru.com",
		Password: "123",
		Keys:     []Key{{Name: "me@tsuru.com-1", Content: "ssh-rsa somekey me@tsuru.com"}},
		APIKey:   "",
	}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer s.conn.Users().Remove(bson.M{"email": u.Email})
	API_Token, err := u.ShowAPIKey()
	c.Assert(API_Token, check.Equals, u.APIKey)
	c.Assert(err, check.IsNil)
}

func (s *S) TestListAllUsers(c *check.C) {
	users, err := ListUsers()
	c.Assert(err, check.IsNil)
	c.Assert(len(users), check.Equals, 1)
}
