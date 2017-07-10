// Copyright 2015 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gandalftest

import (
	"encoding/json"
	"fmt"
	"github.com/tsuru/gandalf/repository"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (s *S) TestNewServerFreePort(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	conn, err := net.Dial("tcp", server.listener.Addr().String())
	c.Assert(err, check.IsNil)
	c.Assert(conn.Close(), check.IsNil)
}

func (s *S) TestNewServerSpecificPort(c *check.C) {
	server, err := NewServer("127.0.0.1:8599")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	conn, err := net.Dial("tcp", server.listener.Addr().String())
	c.Assert(err, check.IsNil)
	c.Assert(conn.Close(), check.IsNil)
}

func (s *S) TestNewServerListenError(c *check.C) {
	listen, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer listen.Close()
	server, err := NewServer(listen.Addr().String())
	c.Assert(err, check.ErrorMatches, `^.*bind: address already in use$`)
	c.Assert(server, check.IsNil)
}

func (s *S) TestServerStop(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	err = server.Stop()
	c.Assert(err, check.IsNil)
	_, err = net.Dial("tcp", server.listener.Addr().String())
	c.Assert(err, check.ErrorMatches, `^.*connection refused$`)
}

func (s *S) TestURL(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	expected := "http://" + server.listener.Addr().String() + "/"
	c.Assert(server.URL(), check.Equals, expected)
}

func (s *S) TestUsers(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.users = []string{"abc", "def"}
	c.Assert(server.Users(), check.DeepEquals, server.users)
}

func (s *S) TestRepositories(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{{Name: "something"}, {Name: "otherthing"}}
	c.Assert(server.Repositories(), check.DeepEquals, server.repos)
}

func (s *S) TestGrants(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{
		{Name: "something", Users: []string{"user1", "user2"}},
		{Name: "otherthing", Users: []string{"user2", "user3"}},
	}
	expected := map[string][]string{
		"something":  {"user1", "user2"},
		"otherthing": {"user2", "user3"},
	}
	c.Assert(server.Grants(), check.DeepEquals, expected)
}

func (s *S) TestReadOnlyGrants(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{
		{Name: "something", ReadOnlyUsers: []string{"user1", "user2"}},
		{Name: "otherthing", ReadOnlyUsers: []string{"user2", "user3"}},
	}
	expected := map[string][]string{
		"something":  {"user1", "user2"},
		"otherthing": {"user2", "user3"},
	}
	c.Assert(server.ReadOnlyGrants(), check.DeepEquals, expected)
}

func (s *S) TestKeys(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.keys = map[string][]key{
		"myuser":   {{Name: "some-key", Body: "some-body"}, {Name: "other", Body: "other-body"}},
		"youruser": nil,
	}
	myKeys, err := server.Keys("myuser")
	c.Assert(err, check.IsNil)
	c.Assert(myKeys, check.DeepEquals, map[string]string{"some-key": "some-body", "other": "other-body"})
	yourKeys, err := server.Keys("youruser")
	c.Assert(err, check.IsNil)
	c.Assert(yourKeys, check.HasLen, 0)
	unknownKeys, err := server.Keys("unknown")
	c.Assert(unknownKeys, check.IsNil)
	c.Assert(err.Error(), check.Equals, "user not found")
}

func (s *S) TestReset(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.users = []string{"user1", "user2"}
	server.keys = map[string][]key{"user1": {{Name: "wat"}}}
	server.repos = []Repository{{Name: "something"}, {Name: "otherthing"}}
	server.PrepareFailure(Failure{Method: "POST", Path: "/user"})
	server.Reset()
	c.Assert(server.users, check.HasLen, 0)
	c.Assert(server.keys, check.HasLen, 0)
	c.Assert(server.repos, check.HasLen, 0)
	c.Assert(server.failures, check.HasLen, 0)
}

func (s *S) TestCreateUser(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"Name":"someuser","Keys":{"rsa":"mykeyrsa"}}`)
	request, _ := http.NewRequest("POST", "/user", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(server.users, check.DeepEquals, []string{"someuser"})
	c.Assert(server.keys, check.DeepEquals, map[string][]key{"someuser": {{Name: "rsa", Body: "mykeyrsa"}}})
}

func (s *S) TestCreateDuplicateUser(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"Name":"someuser","Keys":{"rsa":"mykeyrsa"}}`)
	request, _ := http.NewRequest("POST", "/user", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	recorder = httptest.NewRecorder()
	body = strings.NewReader(`{"Name":"someuser","Keys":{"rsa":"mykeyrsa"}}`)
	request, _ = http.NewRequest("POST", "/user", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(recorder.Body.String(), check.Equals, "user already exists\n")
	c.Assert(server.Users(), check.DeepEquals, []string{"someuser"})
}

func (s *S) TestRemoveUser(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"Name":"someuser","Keys":{"rsa":"mykeyrsa"}}`)
	request, _ := http.NewRequest("POST", "/user", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	recorder = httptest.NewRecorder()
	request, _ = http.NewRequest("DELETE", "/user/someuser", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(server.users, check.HasLen, 0)
	c.Assert(server.keys, check.HasLen, 0)
}

func (s *S) TestRemoveUserNotFound(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("DELETE", "/user/someuser", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "user not found\n")
}

func (s *S) TestCreateRepository(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.Host = "localhost"
	server.users = []string{"user1", "user2", "user3"}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"Name":"myrepo","Users":["user1","user2"],"ReadOnlyUsers":["user3"],"IsPublic":true}`)
	request, _ := http.NewRequest("POST", "/repository", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(server.repos, check.HasLen, 1)
	c.Assert(server.repos[0].Name, check.Equals, "myrepo")
	c.Assert(server.repos[0].Users, check.DeepEquals, []string{"user1", "user2"})
	c.Assert(server.repos[0].ReadOnlyUsers, check.DeepEquals, []string{"user3"})
	c.Assert(server.repos[0].IsPublic, check.Equals, true)
	c.Assert(server.repos[0].ReadOnlyURL, check.Equals, "git://localhost/myrepo.git")
	c.Assert(server.repos[0].ReadWriteURL, check.Equals, "git@localhost:myrepo.git")
}

func (s *S) TestCreateRepositoryDuplicateName(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.users = []string{"user1"}
	server.repos = []Repository{{Name: "somerepo"}}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"Name":"somerepo","Users":["user1"],"IsPublic":false}`)
	request, _ := http.NewRequest("POST", "/repository", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(recorder.Body.String(), check.Equals, "repository already exists\n")
}

func (s *S) TestCreateRepositoryUserNotFound(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"Name":"myrepo","Users":["user1","user2"],"ReadOnlyUsers":["user3"],"IsPublic":true}`)
	request, _ := http.NewRequest("POST", "/repository", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, `user "user1" not found`+"\n")
}

func (s *S) TestCreateRepositoryNoUsers(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"Name":"myrepo","IsPublic":true}`)
	request, _ := http.NewRequest("POST", "/repository", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "missing users\n")
}

func (s *S) TestRemoveRepository(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{{Name: "somerepo"}}
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("DELETE", "/repository/somerepo", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(server.repos, check.HasLen, 0)
}

func (s *S) TestRemoveRepositoryNotFound(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("DELETE", "/repository/somerepo", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "repository not found\n")
}

func (s *S) TestGetRepository(c *check.C) {
	repo := Repository{Name: "somerepo"}
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{repo}
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("GET", "/repository/somerepo", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var got Repository
	err = json.NewDecoder(recorder.Body).Decode(&got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, repo)
}

func (s *S) TestGetRepositoryNotFound(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("GET", "/repository/somerepo", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "repository not found\n")
}

func (s *S) TestGetDiff(c *check.C) {
	repo := Repository{Name: "somerepo", Diffs: make(chan string, 1)}
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{repo}
	server.PrepareDiff(repo.Name, "some diff")
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("GET", "/repository/somerepo/diff/commits?previous_commit=10&last_commit=11", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "some diff")
}

func (s *S) TestGetDiffUnprepared(c *check.C) {
	repo := Repository{Name: "somerepo", Diffs: make(chan string, 1)}
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{repo}
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("GET", "/repository/somerepo/diff/commits?previous_commit=10&last_commit=11", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "")
}

func (s *S) TestGetDiffNotFound(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("GET", "/repository/somerepo/diff/commits?previous_commit=10&last_commit=11", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "repository not found\n")
}

func (s *S) TestAddKeys(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.users = []string{"myuser"}
	server.keys["myuser"] = nil
	recorder := httptest.NewRecorder()
	body := strings.NewReader(fmt.Sprintf(`{"mykey":%q}`, publicKey))
	request, _ := http.NewRequest("POST", "/user/myuser/key", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(server.keys["myuser"], check.DeepEquals, []key{{Name: "mykey", Body: publicKey}})
}

func (s *S) TestAddKeysUserNotFound(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	recorder := httptest.NewRecorder()
	body := strings.NewReader(fmt.Sprintf(`{"mykey":%q}`, publicKey))
	request, _ := http.NewRequest("POST", "/user/myuser/key", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "user not found\n")
}

func (s *S) TestAddKeysDuplicate(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.users = []string{"myuser"}
	server.keys["myuser"] = []key{{Name: "mykey", Body: "irrelevant"}}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(fmt.Sprintf(`{"mykey":%q}`, publicKey))
	request, _ := http.NewRequest("POST", "/user/myuser/key", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusConflict)
	c.Assert(recorder.Body.String(), check.Equals, "Duplicate key\n")
}

func (s *S) TestAddKeysInvalid(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.users = []string{"myuser"}
	server.keys["myuser"] = nil
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"mykey":"some-invalid-key"}`)
	request, _ := http.NewRequest("POST", "/user/myuser/key", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Invalid key\n")
}

func (s *S) TestUpdateKey(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.users = []string{"myuser"}
	server.keys["myuser"] = []key{{Name: "mykey", Body: "irrelevant"}}
	body := strings.NewReader(publicKey)
	request, _ := http.NewRequest("PUT", "/user/myuser/key/mykey", body)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(server.keys["myuser"], check.DeepEquals, []key{{Name: "mykey", Body: publicKey}})
}

func (s *S) TestUpdateKeyInvalid(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.users = []string{"myuser"}
	server.keys["myuser"] = []key{{Name: "mykey", Body: "irrelevant"}}
	body := strings.NewReader("some-invalid-key")
	request, _ := http.NewRequest("PUT", "/user/myuser/key/mykey", body)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "Invalid key\n")
}

func (s *S) TestUpdateKeyUserNotFound(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	body := strings.NewReader(publicKey)
	request, _ := http.NewRequest("PUT", "/user/myuser/key/mykey", body)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "user not found\n")
}

func (s *S) TestUpdateKeyNotFound(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.users = []string{"myuser"}
	server.keys["myuser"] = nil
	body := strings.NewReader(publicKey)
	request, _ := http.NewRequest("PUT", "/user/myuser/key/mykey", body)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Key not found\n")
}

func (s *S) TestRemoveKey(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.users = []string{"myuser"}
	server.keys["myuser"] = []key{{Name: "mykey", Body: "irrelevant"}}
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("DELETE", "/user/myuser/key/mykey", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(server.keys["myuser"], check.HasLen, 0)
}

func (s *S) TestRemoveKeyUserNotFound(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("DELETE", "/user/myuser/key/mykey", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "user not found\n")
}

func (s *S) TestRemoveKeyKeyNotFound(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.users = []string{"myuser"}
	server.keys["myuser"] = []key{{Name: "theirkey", Body: "irrelevant"}}
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("DELETE", "/user/myuser/key/mykey", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "Key not found\n")
}

func (s *S) TestListKeys(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.users = []string{"myuser"}
	server.keys["myuser"] = []key{{Name: "theirkey", Body: "irrelevant"}, {Name: "mykey", Body: "not irrelevant"}}
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("GET", "/user/myuser/keys", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result map[string]string
	expected := map[string]string{"theirkey": "irrelevant", "mykey": "not irrelevant"}
	err = json.NewDecoder(recorder.Body).Decode(&result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
}

func (s *S) TestListKeysUserNotFound(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("GET", "/user/myuser/keys", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "user not found\n")
}

func (s *S) TestGrantAccess(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{{Name: "somerepo", Users: []string{"user1"}}, {Name: "otherrepo"}, {Name: "myrepo"}}
	server.users = []string{"user1", "user2", "user3"}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"users":["user1","user2"],"repositories":["somerepo","myrepo"]}`)
	request, _ := http.NewRequest("POST", "/repository/grant", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(server.repos[0].Users, check.DeepEquals, []string{"user1", "user2"})
	c.Assert(server.repos[1].Users, check.HasLen, 0)
	c.Assert(server.repos[2].Users, check.DeepEquals, []string{"user1", "user2"})
}

func (s *S) TestGrantAccessReadOnly(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{{Name: "somerepo", ReadOnlyUsers: []string{"user1"}}, {Name: "otherrepo"}, {Name: "myrepo"}}
	server.users = []string{"user1", "user2", "user3"}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"users":["user1","user2"],"repositories":["somerepo","myrepo"]}`)
	request, _ := http.NewRequest("POST", "/repository/grant?readonly=yes", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(server.repos[0].ReadOnlyUsers, check.DeepEquals, []string{"user1", "user2"})
	c.Assert(server.repos[1].ReadOnlyUsers, check.HasLen, 0)
	c.Assert(server.repos[2].ReadOnlyUsers, check.DeepEquals, []string{"user1", "user2"})
}

func (s *S) TestGrantAccessUserNotFound(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{{Name: "somerepo", Users: []string{"user1"}}, {Name: "otherrepo"}, {Name: "myrepo"}}
	server.users = []string{"user1", "user2", "user3"}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"users":["user2","user4"],"repositories":["somerepo","myrepo"]}`)
	request, _ := http.NewRequest("POST", "/repository/grant", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, `user "user4" not found`+"\n")
	c.Assert(server.repos[0].Users, check.DeepEquals, []string{"user1"})
	c.Assert(server.repos[1].Users, check.HasLen, 0)
	c.Assert(server.repos[2].Users, check.HasLen, 0)
}

func (s *S) TestGrantAccessRepositoryNotFound(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{{Name: "somerepo", Users: []string{"user1"}}, {Name: "otherrepo"}, {Name: "myrepo"}}
	server.users = []string{"user1", "user2", "user3"}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"users":["user2","user3"],"repositories":["somerepo","watrepo"]}`)
	request, _ := http.NewRequest("POST", "/repository/grant", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, `repository "watrepo" not found`+"\n")
	c.Assert(server.repos[0].Users, check.DeepEquals, []string{"user1"})
	c.Assert(server.repos[1].Users, check.HasLen, 0)
	c.Assert(server.repos[2].Users, check.HasLen, 0)
}

func (s *S) TestGrantAccessRepositoryMissingUsers(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{{Name: "somerepo", ReadOnlyUsers: []string{"user1"}}, {Name: "otherrepo"}, {Name: "myrepo"}}
	server.users = []string{"user1", "user2", "user3"}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"repositories":["somerepo","watrepo"]}`)
	request, _ := http.NewRequest("POST", "/repository/grant", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "missing users\n")
}

func (s *S) TestGrantAccessRepositoryMissingRepositories(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{{Name: "somerepo", ReadOnlyUsers: []string{"user1"}}, {Name: "otherrepo"}, {Name: "myrepo"}}
	server.users = []string{"user1", "user2", "user3"}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"users":["user1","user2"]}`)
	request, _ := http.NewRequest("POST", "/repository/grant", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "missing repositories\n")
}

func (s *S) TestRevokeAccess(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{
		{Name: "somerepo", Users: []string{"user1", "user3"}},
		{Name: "otherrepo", Users: []string{"user2", "user3"}},
		{Name: "myrepo", Users: []string{"user1", "user2"}},
	}
	server.users = []string{"user1", "user2", "user3"}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"users":["user1","user2"],"repositories":["somerepo","myrepo"]}`)
	request, _ := http.NewRequest("DELETE", "/repository/revoke", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(server.repos[0].Users, check.DeepEquals, []string{"user3"})
	c.Assert(server.repos[1].Users, check.DeepEquals, []string{"user2", "user3"})
	c.Assert(server.repos[2].Users, check.HasLen, 0)
}

func (s *S) TestRevokeAccessReadOnly(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{
		{Name: "somerepo", ReadOnlyUsers: []string{"user1", "user3"}},
		{Name: "otherrepo", ReadOnlyUsers: []string{"user2", "user3"}},
		{Name: "myrepo", ReadOnlyUsers: []string{"user1", "user2"}},
	}
	server.users = []string{"user1", "user2", "user3"}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"users":["user1","user2"],"repositories":["somerepo","myrepo"]}`)
	request, _ := http.NewRequest("DELETE", "/repository/revoke?readonly=yes", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(server.repos[0].ReadOnlyUsers, check.DeepEquals, []string{"user3"})
	c.Assert(server.repos[1].ReadOnlyUsers, check.DeepEquals, []string{"user2", "user3"})
	c.Assert(server.repos[2].ReadOnlyUsers, check.HasLen, 0)
}

func (s *S) TestRevokeAccessUserNotFound(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{
		{Name: "somerepo", Users: []string{"user1", "user3"}},
		{Name: "otherrepo", Users: []string{"user2", "user3"}},
		{Name: "myrepo", Users: []string{"user1", "user2"}},
	}
	server.users = []string{"user1", "user2", "user3"}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"users":["user1","user4"],"repositories":["somerepo","myrepo"]}`)
	request, _ := http.NewRequest("DELETE", "/repository/revoke", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, `user "user4" not found`+"\n")
}

func (s *S) TestRevokeAccessRepositoryNotFound(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{
		{Name: "somerepo", Users: []string{"user1", "user3"}},
		{Name: "otherrepo", Users: []string{"user2", "user3"}},
		{Name: "myrepo", Users: []string{"user1", "user2"}},
	}
	server.users = []string{"user1", "user2", "user3"}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"users":["user1","user2"],"repositories":["somerepo","sumrepo"]}`)
	request, _ := http.NewRequest("DELETE", "/repository/revoke", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, `repository "sumrepo" not found`+"\n")
}

func (s *S) TestRevokeAccessMissingUsers(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{
		{Name: "somerepo", Users: []string{"user1", "user3"}},
		{Name: "otherrepo", Users: []string{"user2", "user3"}},
		{Name: "myrepo", Users: []string{"user1", "user2"}},
	}
	server.users = []string{"user1", "user2", "user3"}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"repositories":["somerepo","myrepo"]}`)
	request, _ := http.NewRequest("DELETE", "/repository/revoke", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "missing users\n")
}

func (s *S) TestRevokeAccessMissingRepositories(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{
		{Name: "somerepo", Users: []string{"user1", "user3"}},
		{Name: "otherrepo", Users: []string{"user2", "user3"}},
		{Name: "myrepo", Users: []string{"user1", "user2"}},
	}
	server.users = []string{"user1", "user2", "user3"}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"users":["user1","user2"]}`)
	request, _ := http.NewRequest("DELETE", "/repository/revoke", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "missing repositories\n")
}

func (s *S) TestHealthCheck(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("GET", "/healthcheck", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Body.String(), check.Equals, "WORKING")
}

func (s *S) TestPrepareFailure(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.PrepareFailure(Failure{Method: "POST", Path: "/users", Response: "fatal error"})
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"Name":"someuser","Keys":{"rsa":"mykeyrsa"}}`)
	request, _ := http.NewRequest("POST", "/users", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), check.Equals, "fatal error\n")
}

func (s *S) TestPrepareFailureWithCode(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.PrepareFailure(Failure{Code: http.StatusBadRequest, Method: "POST", Path: "/users", Response: "fatal error"})
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"Name":"someuser","Keys":{"rsa":"mykeyrsa"}}`)
	request, _ := http.NewRequest("POST", "/users", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusBadRequest)
	c.Assert(recorder.Body.String(), check.Equals, "fatal error\n")
}

func (s *S) TestPrepareFailureNotMatching(c *check.C) {
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.PrepareFailure(Failure{Method: "GET", Path: "/users", Response: "fatal error"})
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"Name":"someuser","Keys":{"rsa":"mykeyrsa"}}`)
	request, _ := http.NewRequest("POST", "/users", body)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
}

func (s *S) TestGetLogs(c *check.C) {
	repo := Repository{Name: "somerepo"}
	server, err := NewServer("127.0.0.1:0")
	c.Assert(err, check.IsNil)
	defer server.Stop()
	server.repos = []Repository{repo}
	expected := repository.GitHistory{
		Commits: []repository.GitLog{
			{
				Subject: "mymsg",
			},
		},
	}
	server.PrepareLogs(repo.Name, expected)
	recorder := httptest.NewRecorder()
	request, _ := http.NewRequest("GET", "/repository/somerepo/logs?ref=x", nil)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var result repository.GitHistory
	err = json.Unmarshal(recorder.Body.Bytes(), &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
}

const publicKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDD91CO+YIU6nIb+l+JewPMLbUB9IZx4g6IUuqyLbmCi+8DNliEjE/KWUISPlkPWoDK4ibEY/gZPLPRMT3acA+2cAf3uApBwegvDgtDv1lgtTbkMc8QJaT044Vg+JtVDFraXU4T8fn/apVMMXro0Kr/DaLzUsxSigGrCIRyT1vkMCnya8oaQHu1Qa/wnOjd6tZzvzIsxJirAbQvzlLOb89c7LTPhUByySTQmgSnoNR6ZdPpjDwnaQgyAjbsPKjhkQ1AkcxOxBi0GwwSCO7aZ+T3F/mJ1bUhEE5BMh+vO3HQ3gGkc1xeQW4H7ZL33sJkP0Tb9zslaE1lT+fuOi7NBUK5 f@somewhere"
