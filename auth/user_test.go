// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/repository/repositorytest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func (s *S) TestCreateUser(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	var result User
	collection := s.conn.Users()
	err = collection.Find(bson.M{"email": u.Email}).One(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result.Email, check.Equals, u.Email)
	c.Assert(repositorytest.Users(), check.DeepEquals, []string{u.Email})
}

func (s *S) TestCreateUserReturnsErrorWhenTryingToCreateAUserWithDuplicatedEmail(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	err = u.Create()
	c.Assert(err, check.NotNil)
}

func (s *S) TestGetUserByEmail(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
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
	defer u.Delete()
	u.Password = "1234"
	err = u.Update()
	c.Assert(err, check.IsNil)
	u2, err := GetUserByEmail("wolverine@xmen.com")
	c.Assert(err, check.IsNil)
	c.Assert(u2.Password, check.Equals, "1234")
}

func (s *S) TestDeleteUser(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	err = u.Delete()
	c.Assert(err, check.IsNil)
	user, err := GetUserByEmail(u.Email)
	c.Assert(err, check.Equals, ErrUserNotFound)
	c.Assert(user, check.IsNil)
	c.Assert(repositorytest.Users(), check.HasLen, 0)
}

func (s *S) TestAddKeyAddsAKeyToTheUser(c *check.C) {
	u := &User{Email: "sacefulofsecrets@pinkfloyd.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	key := repository.Key{Name: "some-key", Body: "my-key"}
	err = u.AddKey(key)
	c.Assert(err, check.IsNil)
	keys, err := repository.Manager().(repository.KeyRepositoryManager).ListKeys(u.Email)
	c.Assert(err, check.IsNil)
	c.Assert(keys, check.DeepEquals, []repository.Key{key})
}

func (s *S) TestAddKeyEmptyName(c *check.C) {
	u := &User{Email: "sacefulofsecrets@pinkfloyd.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	key := repository.Key{Body: "my-key"}
	err = u.AddKey(key)
	c.Assert(err, check.Equals, ErrInvalidKey)
}

func (s *S) TestAddDuplicatedKey(c *check.C) {
	u := &User{Email: "sacefulofsecrets@pinkfloyd.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	key := repository.Key{Name: "my-key", Body: "other-key"}
	repository.Manager().(repository.KeyRepositoryManager).AddKey(u.Email, key)
	err = u.AddKey(key)
	c.Assert(err, check.Equals, repository.ErrKeyAlreadyExists)
}

func (s *S) TestRemoveKeyRemovesAKeyFromTheUser(c *check.C) {
	key := repository.Key{Body: "my-key", Name: "the-key"}
	u := &User{Email: "shineon@pinkfloyd.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	err = repository.Manager().(repository.KeyRepositoryManager).AddKey(u.Email, key)
	c.Assert(err, check.IsNil)
	err = u.RemoveKey(repository.Key{Name: "the-key"})
	c.Assert(err, check.IsNil)
	keys, err := repository.Manager().(repository.KeyRepositoryManager).ListKeys(u.Email)
	c.Assert(err, check.IsNil)
	c.Assert(keys, check.HasLen, 0)
}

func (s *S) TestRemoveUnknownKey(c *check.C) {
	u := &User{Email: "shine@pinkfloyd.com"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	err = u.RemoveKey(repository.Key{Body: "my-key"})
	c.Assert(err, check.Equals, repository.ErrKeyNotFound)
}

func (s *S) TestTeams(c *check.C) {
	u := User{Email: "me@tsuru.com", Password: "123"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
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
		s.conn.Apps().RemoveAll(bson.M{"name": bson.M{"$in": []string{a.Name, a2.Name}}})
		s.conn.Teams().RemoveId(team.Name)
	}()
	aApps, err := s.user.AllowedApps()
	c.Assert(aApps, check.DeepEquals, []string{a.Name, a2.Name})
}

func (s *S) TestListKeysShouldGetKeysFromTheRepositoryManager(c *check.C) {
	u := User{
		Email:    "wolverine@xmen.com",
		Password: "123456",
	}
	newKeys := []repository.Key{{Name: "key1", Body: "superkey"}, {Name: "key2", Body: "hiperkey"}}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	repository.Manager().(repository.KeyRepositoryManager).AddKey(u.Email, newKeys[0])
	repository.Manager().(repository.KeyRepositoryManager).AddKey(u.Email, newKeys[1])
	keys, err := u.ListKeys()
	c.Assert(err, check.IsNil)
	expected := map[string]string{"key1": "superkey", "key2": "hiperkey"}
	c.Assert(keys, check.DeepEquals, expected)
}

func (s *S) TestListKeysRepositoryManagerFailure(c *check.C) {
	u := User{Email: "wolverine@xmen.com", Password: "123456"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	err = repository.Manager().RemoveUser(u.Email)
	c.Assert(err, check.IsNil)
	keys, err := u.ListKeys()
	c.Assert(keys, check.HasLen, 0)
	c.Assert(err.Error(), check.Equals, "user not found")
}

func (s *S) TestShowAPIKeyWhenAPITokenAlreadyExists(c *check.C) {
	u := User{
		Email:    "me@tsuru.com",
		Password: "123",
		APIKey:   "1ioudh8ydb2idn1ehnpoqwjmhdjqwz12po1",
	}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	API_Token, err := u.ShowAPIKey()
	c.Assert(API_Token, check.Equals, u.APIKey)
	c.Assert(err, check.IsNil)
}

func (s *S) TestShowAPIKeyWhenAPITokenNotExists(c *check.C) {
	u := User{Email: "me@tsuru.com", Password: "123"}
	err := u.Create()
	c.Assert(err, check.IsNil)
	defer u.Delete()
	API_Token, err := u.ShowAPIKey()
	c.Assert(API_Token, check.Equals, u.APIKey)
	c.Assert(err, check.IsNil)
}

func (s *S) TestListAllUsers(c *check.C) {
	users, err := ListUsers()
	c.Assert(err, check.IsNil)
	c.Assert(len(users), check.Equals, 1)
}
