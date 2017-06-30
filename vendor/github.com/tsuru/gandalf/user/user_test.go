// Copyright 2015 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package user

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/gandalf/db"
	"github.com/tsuru/gandalf/fs"
	"github.com/tsuru/gandalf/repository"
	"github.com/tsuru/tsuru/fs/fstest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	rfs *fstest.RecordingFs
}

var _ = check.Suite(&S{})

func (s *S) authKeysContent(c *check.C) string {
	authFile := path.Join(os.Getenv("HOME"), ".ssh", "authorized_keys")
	f, err := fs.Filesystem().OpenFile(authFile, os.O_RDWR, 0755)
	c.Assert(err, check.IsNil)
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	c.Assert(err, check.IsNil)
	return string(b)
}

func (s *S) SetUpSuite(c *check.C) {
	err := config.ReadConfigFile("../etc/gandalf.conf")
	c.Check(err, check.IsNil)
	config.Set("database:name", "gandalf_user_tests")
}

func (s *S) SetUpTest(c *check.C) {
	s.rfs = &fstest.RecordingFs{}
	fs.Fsystem = s.rfs
}

func (s *S) TearDownTest(c *check.C) {
	s.rfs.Remove(authKey())
}

func (s *S) TearDownSuite(c *check.C) {
	fs.Fsystem = nil
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.User().Database.DropDatabase()
}

func (s *S) TestNewUserReturnsAStructFilled(c *check.C) {
	u, err := New("someuser", map[string]string{"somekey": rawKey})
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.User().Remove(bson.M{"_id": u.Name})
	defer conn.Key().Remove(bson.M{"name": "somekey"})
	c.Assert(u.Name, check.Equals, "someuser")
	var key Key
	err = conn.Key().Find(bson.M{"name": "somekey"}).One(&key)
	c.Assert(err, check.IsNil)
	c.Assert(key.Name, check.Equals, "somekey")
	c.Assert(key.Body, check.Equals, body)
	c.Assert(key.Comment, check.Equals, comment)
	c.Assert(key.UserName, check.Equals, u.Name)
}

func (s *S) TestNewDuplicateUser(c *check.C) {
	u, err := New("someuser", map[string]string{"somekey": rawKey})
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.User().Remove(bson.M{"_id": u.Name})
	defer conn.Key().Remove(bson.M{"name": "somekey"})
	_, err = New("someuser", map[string]string{"somekey": rawKey})
	c.Assert(err, check.Equals, ErrUserAlreadyExists)
}

func (s *S) TestNewDuplicateUserDifferentKey(c *check.C) {
	u, err := New("someuser", map[string]string{"somekey": rawKey})
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.User().Remove(bson.M{"_id": u.Name})
	defer conn.Key().Remove(bson.M{"name": "somekey"})
	_, err = New("someuser", map[string]string{"somedifferentkey": rawKey + "fakeKey"})
	c.Assert(err, check.Equals, ErrUserAlreadyExists)
}

func (s *S) TestNewUserShouldFailWhenMongoDbIsDown(c *check.C) {
	oldURL, _ := config.Get("database:url")
	defer config.Set("database:url", oldURL)
	config.Unset("database:url")
	config.Set("database:url", "invalid")
	_, err := New("someuser", map[string]string{"somekey": rawKey})
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to connect to MongoDB of Gandalf \"invalid\" - no reachable servers.")
}

func (s *S) TestNewUserShouldStoreUserInDatabase(c *check.C) {
	u, err := New("someuser", map[string]string{"somekey": rawKey})
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.User().Remove(bson.M{"_id": u.Name})
	defer conn.Key().Remove(bson.M{"name": "somekey"})
	err = conn.User().FindId(u.Name).One(&u)
	c.Assert(err, check.IsNil)
	c.Assert(u.Name, check.Equals, "someuser")
	n, err := conn.Key().Find(bson.M{"name": "somekey"}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(n, check.Equals, 1)
}

func (s *S) TestNewChecksIfUserIsValidBeforeStoring(c *check.C) {
	_, err := New("", map[string]string{})
	c.Assert(err, check.NotNil)
	e, ok := err.(*InvalidUserError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.message, check.Equals, "username is not valid")
}

func (s *S) TestNewWritesKeyInAuthorizedKeys(c *check.C) {
	u, err := New("piccolo", map[string]string{"somekey": rawKey})
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.User().Remove(bson.M{"_id": u.Name})
	defer conn.Key().Remove(bson.M{"name": "somekey"})
	var key Key
	err = conn.Key().Find(bson.M{"name": "somekey"}).One(&key)
	c.Assert(err, check.IsNil)
	keys := s.authKeysContent(c)
	c.Assert(keys, check.Equals, key.format())
}

func (s *S) TestIsValid(c *check.C) {
	var tests = []struct {
		input    string
		expected bool
	}{
		{"", false},
		{"r2d2@gmail.com", true},
		{"r2-d2@gmail.com", true},
		{"r2d2+tsuru@gmail.com", true},
		{"r2d2", true},
		{"gopher", true},
		{"go-pher", true},
	}
	for _, t := range tests {
		u := User{Name: t.input}
		v, _ := u.isValid()
		if v != t.expected {
			c.Errorf("Is %q valid? Want %v. Got %v.", t.input, t.expected, v)
		}
	}
}

func (s *S) TestRemove(c *check.C) {
	u, err := New("someuser", map[string]string{})
	c.Assert(err, check.IsNil)
	err = Remove(u.Name)
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	lenght, err := conn.User().FindId(u.Name).Count()
	c.Assert(err, check.IsNil)
	c.Assert(lenght, check.Equals, 0)
}

func (s *S) TestRemoveRemovesKeyFromAuthorizedKeysFile(c *check.C) {
	u, err := New("gandalf", map[string]string{"somekey": rawKey})
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Key().Remove(bson.M{"name": "somekey"})
	err = Remove(u.Name)
	c.Assert(err, check.IsNil)
	got := s.authKeysContent(c)
	c.Assert(got, check.Equals, "")
}

func (s *S) TestRemoveNotFound(c *check.C) {
	err := Remove("otheruser")
	c.Assert(err, check.Equals, ErrUserNotFound)
}

func (s *S) TestRemoveDoesNotRemovesUserWhenUserIsTheOnlyOneAssciatedWithOneRepository(c *check.C) {
	u, err := New("silver", map[string]string{})
	c.Assert(err, check.IsNil)
	r := s.createRepo("run", []string{u.Name}, c)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Repository().Remove(bson.M{"_id": r.Name})
	defer conn.User().Remove(bson.M{"_id": u.Name})
	err = Remove(u.Name)
	c.Assert(err, check.ErrorMatches, "^Could not remove user: user is the only one with access to at least one of it's repositories$")
}

func (s *S) TestRemoveRevokesAccessToReposWithMoreThanOneUserAssociated(c *check.C) {
	u, r, r2 := s.userPlusRepos(c)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Repository().Remove(bson.M{"_id": r.Name})
	defer conn.Repository().Remove(bson.M{"_id": r2.Name})
	defer conn.User().Remove(bson.M{"_id": u.Name})
	err = Remove(u.Name)
	c.Assert(err, check.IsNil)
	s.retrieveRepos(r, r2, c)
	c.Assert(r.Users, check.DeepEquals, []string{"slot"})
	c.Assert(r2.Users, check.DeepEquals, []string{"cnot"})
}

func (s *S) retrieveRepos(r, r2 *repository.Repository, c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	err = conn.Repository().FindId(r.Name).One(&r)
	c.Assert(err, check.IsNil)
	err = conn.Repository().FindId(r2.Name).One(&r2)
	c.Assert(err, check.IsNil)
}

func (s *S) userPlusRepos(c *check.C) (*User, *repository.Repository, *repository.Repository) {
	u, err := New("silver", map[string]string{})
	c.Assert(err, check.IsNil)
	r := s.createRepo("run", []string{u.Name, "slot"}, c)
	r2 := s.createRepo("stay", []string{u.Name, "cnot"}, c)
	return u, &r, &r2
}

func (s *S) createRepo(name string, users []string, c *check.C) repository.Repository {
	r := repository.Repository{Name: name, Users: users}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	err = conn.Repository().Insert(&r)
	c.Assert(err, check.IsNil)
	return r
}

func (s *S) TestHandleAssociatedRepositoriesShouldRevokeAccessToRepoWithMoreThanOneUserAssociated(c *check.C) {
	u, r, r2 := s.userPlusRepos(c)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Repository().RemoveId(r.Name)
	defer conn.Repository().RemoveId(r2.Name)
	defer conn.User().RemoveId(u.Name)
	err = u.handleAssociatedRepositories()
	c.Assert(err, check.IsNil)
	s.retrieveRepos(r, r2, c)
	c.Assert(r.Users, check.DeepEquals, []string{"slot"})
	c.Assert(r2.Users, check.DeepEquals, []string{"cnot"})
}

func (s *S) TestHandleAssociateRepositoriesReturnsErrorWhenUserIsOnlyOneWithAccessToAtLeastOneRepo(c *check.C) {
	u, err := New("umi", map[string]string{})
	c.Assert(err, check.IsNil)
	r := s.createRepo("proj1", []string{"umi"}, c)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.User().RemoveId(u.Name)
	defer conn.Repository().RemoveId(r.Name)
	err = u.handleAssociatedRepositories()
	expected := "^Could not remove user: user is the only one with access to at least one of it's repositories$"
	c.Assert(err, check.ErrorMatches, expected)
}

func (s *S) TestAddKeyShouldSaveTheKeyInTheDatabase(c *check.C) {
	u, err := New("umi", map[string]string{})
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.User().RemoveId(u.Name)
	k := map[string]string{"somekey": rawKey}
	err = AddKey("umi", k)
	c.Assert(err, check.IsNil)
	defer conn.Key().Remove(bson.M{"name": "somekey"})
	var key Key
	err = conn.Key().Find(bson.M{"name": "somekey"}).One(&key)
	c.Assert(err, check.IsNil)
	c.Assert(key.Name, check.Equals, "somekey")
	c.Assert(key.Body, check.Equals, body)
	c.Assert(key.Comment, check.Equals, comment)
	c.Assert(key.UserName, check.Equals, u.Name)
}

func (s *S) TestAddKeyShouldWriteKeyInAuthorizedKeys(c *check.C) {
	u, err := New("umi", map[string]string{})
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.User().RemoveId(u.Name)
	defer conn.Key().Remove(bson.M{"name": "somekey"})
	k := map[string]string{"somekey": rawKey}
	err = AddKey("umi", k)
	c.Assert(err, check.IsNil)
	var key Key
	err = conn.Key().Find(bson.M{"name": "somekey"}).One(&key)
	content := s.authKeysContent(c)
	c.Assert(content, check.Equals, key.format())
}

func (s *S) TestAddKeyShouldReturnCustomErrorWhenUserDoesNotExist(c *check.C) {
	err := AddKey("umi", map[string]string{"somekey": "ssh-rsa mykey umi@host"})
	c.Assert(err, check.Equals, ErrUserNotFound)
}

func (s *S) TestUserUpdateKey(c *check.C) {
	u, err := New("umi", map[string]string{})
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.User().RemoveId(u.Name)
	defer conn.Key().Remove(bson.M{"name": "somekey"})
	k := map[string]string{"somekey": rawKey}
	err = AddKey("umi", k)
	c.Assert(err, check.IsNil)
	newKey := Key{Name: "somekey", Body: otherKey}
	err = UpdateKey("umi", newKey)
	c.Assert(err, check.IsNil)
	content := s.authKeysContent(c)
	newKey.UserName = "umi"
	c.Assert(content, check.Equals, newKey.format())
}

func (s *S) TestUpdateKeyUserNotFound(c *check.C) {
	newKey := Key{Name: "somekey", Body: otherKey}
	err := UpdateKey("umi", newKey)
	c.Assert(err, check.Equals, ErrUserNotFound)
}

func (s *S) TestUserUpdateKeyNotFound(c *check.C) {
	newKey := Key{Name: "somekey", Body: otherKey}
	u, err := New("umi", map[string]string{})
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.User().RemoveId(u.Name)
	err = UpdateKey("umi", newKey)
	c.Assert(err, check.Equals, ErrKeyNotFound)
}

func (s *S) TestRemoveKeyShouldRemoveKeyFromTheDatabase(c *check.C) {
	u, err := New("luke", map[string]string{"homekey": rawKey})
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.User().RemoveId(u.Name)
	err = RemoveKey("luke", "homekey")
	c.Assert(err, check.IsNil)
	count, err := conn.Key().Find(bson.M{"name": "homekey", "username": u.Name}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
}

func (s *S) TestRemoveKeyShouldRemoveFromAuthorizedKeysFile(c *check.C) {
	u, err := New("luke", map[string]string{"homekey": rawKey})
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.User().RemoveId(u.Name)
	defer conn.Key().Remove(bson.M{"name": "homekey"})
	err = RemoveKey("luke", "homekey")
	c.Assert(err, check.IsNil)
	content := s.authKeysContent(c)
	c.Assert(content, check.Equals, "")
}

func (s *S) TestRemoveUnknownKeyFromUser(c *check.C) {
	u, err := New("luke", map[string]string{})
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.User().RemoveId(u.Name)
	err = RemoveKey("luke", "homekey")
	c.Assert(err, check.Equals, ErrKeyNotFound)
}

func (s *S) TestRemoveKeyShouldReturnFormatedErrorMsgWhenUserDoesNotExist(c *check.C) {
	err := RemoveKey("luke", "homekey")
	c.Assert(err, check.Equals, ErrUserNotFound)
}
