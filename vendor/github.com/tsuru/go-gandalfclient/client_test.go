// Copyright 2015 go-gandalfclient authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gandalf

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"time"

	"gopkg.in/check.v1"
)

type unmarshable struct{}

func (u unmarshable) MarshalJSON() ([]byte, error) {
	return nil, errors.New("Unmarshable.")
}

func (s *S) TestDoRequest(c *check.C) {
	h := testHandler{content: `some return message`}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL + "/"}
	body := bytes.NewBufferString(`{"foo":"bar"}`)
	response, err := client.doRequest("POST", "/test", body)
	c.Assert(err, check.IsNil)
	c.Assert(response.StatusCode, check.Equals, 200)
	c.Assert(string(h.body), check.Equals, `{"foo":"bar"}`)
	c.Assert(h.url, check.Equals, "/test")
}

func (s *S) TestDoRequestShouldNotSetContentTypeToJsonWhenBodyIsNil(c *check.C) {
	h := testHandler{content: `some return message`}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	response, err := client.doRequest("DELETE", "/test", nil)
	c.Assert(err, check.IsNil)
	c.Assert(response.StatusCode, check.Equals, 200)
	c.Assert(h.header.Get("Content-Type"), check.Not(check.Equals), "application/json")
}

func (s *S) TestDoRequestConnectionError(c *check.C) {
	client := Client{Endpoint: "http://127.0.0.1:747399"}
	response, err := client.doRequest("GET", "/", nil)
	c.Assert(response, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to connect to Gandalf server (http://127.0.0.1:747399) - Get http://127.0.0.1:747399/: dial tcp: invalid port 747399")
}

func (s *S) TestPost(c *check.C) {
	h := testHandler{content: `some return message`}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	r := repository{Name: "test", Users: []string{"samwan"}}
	err := client.post(r, "/repository")
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/repository")
	c.Assert(h.method, check.Equals, "POST")
	c.Assert(string(h.body), check.Equals, `{"name":"test","users":["samwan"],"ispublic":false}`)
}

func (s *S) TestPostWithError(c *check.C) {
	h := errorHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	r := repository{Name: "test", Users: []string{"samwan"}}
	err := client.post(r, "/repository")
	c.Assert(err, check.ErrorMatches, "^Error performing requested operation\n$")
}

func (s *S) TestPostConnectionFailure(c *check.C) {
	client := Client{Endpoint: "http://127.0.0.1:747399"}
	err := client.post(nil, "/")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to connect to Gandalf server (http://127.0.0.1:747399) - Post http://127.0.0.1:747399/: dial tcp: invalid port 747399")
}

func (s *S) TestPostMarshalingFailure(c *check.C) {
	client := Client{Endpoint: "http://127.0.0.1:747399"}
	err := client.post(unmarshable{}, "/users/something")
	c.Assert(err, check.NotNil)
	e, ok := err.(*json.MarshalerError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Err.Error(), check.Equals, "Unmarshable.")
}

func (s *S) TestPut(c *check.C) {
	h := testHandler{content: `some return message`}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	err := client.put("ssh-key", "/repository")
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/repository")
	c.Assert(h.method, check.Equals, "PUT")
	c.Assert(string(h.body), check.Equals, `ssh-key`)
}

func (s *S) TestPutWithError(c *check.C) {
	h := errorHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	err := client.put("ssh-key", "/repository")
	c.Assert(err, check.ErrorMatches, "^Error performing requested operation\n$")
}

func (s *S) TestPutConnectionFailure(c *check.C) {
	client := Client{Endpoint: "http://127.0.0.1:747399"}
	err := client.put("", "/")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to connect to Gandalf server (http://127.0.0.1:747399) - Put http://127.0.0.1:747399/: dial tcp: invalid port 747399")
}

func (s *S) TestDelete(c *check.C) {
	h := testHandler{content: `some return message`}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	err := client.delete(nil, "/user/someuser")
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/user/someuser")
	c.Assert(h.method, check.Equals, "DELETE")
	c.Assert(string(h.body), check.Equals, "null")
}

func (s *S) TestDeleteWithConnectionError(c *check.C) {
	client := Client{Endpoint: "http://127.0.0.1:747399"}
	err := client.delete(nil, "/users/something")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Failed to connect to Gandalf server (http://127.0.0.1:747399) - dial tcp: invalid port 747399")
}

func (s *S) TestDeleteWithMarshalingError(c *check.C) {
	client := Client{Endpoint: "http://127.0.0.1:747399"}
	err := client.delete(unmarshable{}, "/users/something")
	c.Assert(err, check.NotNil)
	e, ok := err.(*json.MarshalerError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Err.Error(), check.Equals, "Unmarshable.")
}

func (s *S) TestDeleteWithResponseError(c *check.C) {
	h := errorHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	err := client.delete(nil, "/user/someuser")
	c.Assert(err, check.ErrorMatches, "^Error performing requested operation\n$")
	c.Assert(string(h.body), check.Equals, "null")
}

func (s *S) TestDeleteWithBody(c *check.C) {
	h := testHandler{content: `some return message`}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	err := client.delete(map[string]string{"test": "foo"}, "/user/someuser")
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/user/someuser")
	c.Assert(h.method, check.Equals, "DELETE")
	c.Assert(string(h.body), check.Equals, `{"test":"foo"}`)
}

func (s *S) TestGet(c *check.C) {
	h := testHandler{content: `{"fookey": "bar keycontent"}`}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	out, err := client.get("/user/someuser")
	c.Assert(err, check.IsNil)
	c.Assert(string(out), check.Equals, `{"fookey": "bar keycontent"}`)
	c.Assert(h.url, check.Equals, "/user/someuser")
	c.Assert(h.method, check.Equals, "GET")
}

func (s *S) TestGetWithError(c *check.C) {
	h := errorHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	_, err := client.get("/user/someuser")
	c.Assert(err, check.ErrorMatches, "^Error performing requested operation\n$")
}

func (s *S) TestFormatBody(c *check.C) {
	b, err := (&Client{}).formatBody(map[string]string{"test": "foo"})
	c.Assert(err, check.IsNil)
	c.Assert(b.String(), check.Equals, `{"test":"foo"}`)
}

func (s *S) TestFormatBodyReturnJsonNullWithNilBody(c *check.C) {
	b, err := (&Client{}).formatBody(nil)
	c.Assert(err, check.IsNil)
	c.Assert(b.String(), check.Equals, "null")
}

func (s *S) TestFormatBodyMarshalingFailure(c *check.C) {
	client := &Client{}
	b, err := client.formatBody(unmarshable{})
	c.Assert(b, check.IsNil)
	c.Assert(err, check.NotNil)
	e, ok := err.(*json.MarshalerError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.Err.Error(), check.Equals, "Unmarshable.")
}

func (s *S) TestNewRepository(c *check.C) {
	h := testHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	_, err := client.NewRepository("proj1", []string{"someuser"}, false)
	c.Assert(err, check.IsNil)
	c.Assert(string(h.body), check.Equals, `{"name":"proj1","users":["someuser"],"ispublic":false}`)
	c.Assert(h.url, check.Equals, "/repository")
	c.Assert(h.method, check.Equals, "POST")
}

func (s *S) TestNewRepositoryWithError(c *check.C) {
	h := errorHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	_, err := client.NewRepository("proj1", []string{"someuser"}, false)
	expected := "^Error performing requested operation\n$"
	c.Assert(err, check.ErrorMatches, expected)
}

func (s *S) TestGetRepository(c *check.C) {
	content := `{"name":"repo-name","git_url":"git@test.com:repo-name.git","ssh_url":"git://test.com/repo-name.git"}`
	h := testHandler{content: content}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	r, err := client.GetRepository("repo-name")
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/repository/repo-name?:name=repo-name")
	c.Assert(h.method, check.Equals, "GET")
	c.Assert(r.Name, check.Equals, "repo-name")
	c.Assert(r.GitURL, check.Equals, "git@test.com:repo-name.git")
	c.Assert(r.SshURL, check.Equals, "git://test.com/repo-name.git")
}

func (s *S) TestGetRepositoryOnUnmarshalError(c *check.C) {
	h := testHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	r, err := client.GetRepository("repo-name")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^Caught error decoding returned json: unexpected end of JSON input$")
	c.Assert(r.Name, check.Equals, "")
	c.Assert(r.GitURL, check.Equals, "")
	c.Assert(r.SshURL, check.Equals, "")
}

func (s *S) TestGetRepositoryOnHTTPError(c *check.C) {
	content := `null`
	h := errorHandler{content: content}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	_, err := client.GetRepository("repo-name")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^Error performing requested operation\n$")
}

func (s *S) TestNewUser(c *check.C) {
	h := testHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	_, err := client.NewUser("someuser", map[string]string{"testkey": "ssh-rsa somekey"})
	c.Assert(err, check.IsNil)
	c.Assert(string(h.body), check.Equals, `{"name":"someuser","keys":{"testkey":"ssh-rsa somekey"}}`)
	c.Assert(h.url, check.Equals, "/user")
	c.Assert(h.method, check.Equals, "POST")
}

func (s *S) TestNewUserWithError(c *check.C) {
	h := errorHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	_, err := client.NewUser("someuser", map[string]string{"testkey": "ssh-rsa somekey"})
	expected := "^Error performing requested operation\n$"
	c.Assert(err, check.ErrorMatches, expected)
}

func (s *S) TestRemoveUser(c *check.C) {
	h := testHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	err := client.RemoveUser("someuser")
	c.Assert(err, check.IsNil)
	c.Assert(string(h.body), check.Equals, "null")
	c.Assert(h.url, check.Equals, "/user/someuser")
	c.Assert(h.method, check.Equals, "DELETE")
}

func (s *S) TestRemoveUserWithError(c *check.C) {
	h := errorHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	err := client.RemoveUser("someuser")
	expected := "^Error performing requested operation\n$"
	c.Assert(err, check.ErrorMatches, expected)
}

func (s *S) TestRemoveRepository(c *check.C) {
	h := testHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	err := client.RemoveRepository("project1")
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/repository/project1")
	c.Assert(h.method, check.Equals, "DELETE")
	c.Assert(string(h.body), check.Equals, "null")
}

func (s *S) TestRemoveRepositoryWithError(c *check.C) {
	h := errorHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	err := client.RemoveRepository("proj2")
	expected := "^Error performing requested operation\n$"
	c.Assert(err, check.ErrorMatches, expected)
}

func (s *S) TestAddKey(c *check.C) {
	h := testHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	key := map[string]string{"pubkey": "ssh-rsa somekey me@myhost"}
	err := client.AddKey("username", key)
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/user/username/key")
	c.Assert(h.method, check.Equals, "POST")
	c.Assert(string(h.body), check.Equals, `{"pubkey":"ssh-rsa somekey me@myhost"}`)
	c.Assert(h.header.Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestAddKeyWithError(c *check.C) {
	h := errorHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	err := client.AddKey("proj2", map[string]string{"key": "ssh-rsa keycontent user@host"})
	expected := "^Error performing requested operation\n$"
	c.Assert(err, check.ErrorMatches, expected)
}

func (s *S) TestUpdateKey(c *check.C) {
	h := testHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	err := client.UpdateKey("username", "pubkey", "ssh-rsa somekey me@myhost")
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/user/username/key/pubkey")
	c.Assert(h.method, check.Equals, "PUT")
	c.Assert(string(h.body), check.Equals, "ssh-rsa somekey me@myhost")
	c.Assert(h.header.Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestRemoveKey(c *check.C) {
	h := testHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	err := client.RemoveKey("username", "keyname")
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/user/username/key/keyname")
	c.Assert(h.method, check.Equals, "DELETE")
	c.Assert(string(h.body), check.Equals, "null")
}

func (s *S) TestRemoveKeyWithError(c *check.C) {
	h := errorHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	err := client.RemoveKey("proj2", "keyname")
	expected := "^Error performing requested operation\n$"
	c.Assert(err, check.ErrorMatches, expected)
}

func (s *S) TestListKeys(c *check.C) {
	h := testHandler{content: `{"fookey":"bar keycontent"}`}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	keys, err := client.ListKeys("userx")
	c.Assert(err, check.IsNil)
	expected := map[string]string{"fookey": "bar keycontent"}
	c.Assert(expected, check.DeepEquals, keys)
	c.Assert(h.url, check.Equals, "/user/userx/keys")
	c.Assert(h.method, check.Equals, "GET")
}

func (s *S) TestListKeysWithError(c *check.C) {
	h := errorHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	_, err := client.ListKeys("userx")
	c.Assert(err.Error(), check.Equals, "Error performing requested operation\n")
}

func (s *S) TestGrantAccess(c *check.C) {
	h := testHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	repositories := []string{"projectx", "projecty"}
	users := []string{"userx"}
	err := client.GrantAccess(repositories, users)
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/repository/grant")
	c.Assert(h.method, check.Equals, "POST")
	expected, err := json.Marshal(map[string][]string{"repositories": repositories, "users": users})
	c.Assert(err, check.IsNil)
	c.Assert(h.body, check.DeepEquals, expected)
	c.Assert(h.header.Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestGrantAccessWithError(c *check.C) {
	h := errorHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	err := client.GrantAccess([]string{"projectx", "projecty"}, []string{"userx"})
	expected := "^Error performing requested operation\n$"
	c.Assert(err, check.ErrorMatches, expected)
}

func (s *S) TestRevokeAccess(c *check.C) {
	h := testHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	repositories := []string{"projectx", "projecty"}
	users := []string{"userx"}
	err := client.RevokeAccess(repositories, users)
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/repository/revoke")
	c.Assert(h.method, check.Equals, "DELETE")
	expected, err := json.Marshal(map[string][]string{"repositories": repositories, "users": users})
	c.Assert(err, check.IsNil)
	c.Assert(h.body, check.DeepEquals, expected)
	c.Assert(h.header.Get("Content-Type"), check.Equals, "application/json")
}

func (s *S) TestRevokeAccessWithError(c *check.C) {
	h := errorHandler{}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	err := client.RevokeAccess([]string{"projectx", "projecty"}, []string{"usery"})
	expected := "^Error performing requested operation\n$"
	c.Assert(err, check.ErrorMatches, expected)
}

func (s *S) TestGetDiff(c *check.C) {
	content := "diff_test"
	h := testHandler{content: content}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	diffOutput, err := client.GetDiff("repo-name", "1b970b076bbb30d708e262b402d4e31910e1dc10", "545b1904af34458704e2aa06ff1aaffad5289f8f")
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/repository/repo-name/diff/commits?:name=repo-name&previous_commit=1b970b076bbb30d708e262b402d4e31910e1dc10&last_commit=545b1904af34458704e2aa06ff1aaffad5289f8f")
	c.Assert(h.method, check.Equals, "GET")
	c.Assert(diffOutput, check.Equals, content)
}

func (s *S) TestGetDiffOnHTTPError(c *check.C) {
	content := `null`
	h := errorHandler{content: content}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	_, err := client.GetDiff("repo-name", "1b970b076bbb30d708e262b402d4e31910e1dc10", "545b1904af34458704e2aa06ff1aaffad5289f8f")
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^Caught error getting repository metadata: Error performing requested operation\n$")
}

func (s *S) TestHealthCheck(c *check.C) {
	content := "test"
	h := testHandler{content: content}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	result, err := client.GetHealthCheck()
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/healthcheck")
	c.Assert(h.method, check.Equals, "GET")
	c.Assert(string(result), check.Equals, content)
}

func (s *S) TestHealthCheckOnHTTPError(c *check.C) {
	content := `null`
	h := errorHandler{content: content}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	_, err := client.GetHealthCheck()
	c.Assert(err, check.NotNil)
	c.Assert(err, check.ErrorMatches, "^Error performing requested operation\n$")
}

func (s *S) TestGetLog(c *check.C) {
	content := `
	{
	    "commits": [
	        {
	            "ref": "30f221131c7d6ca50af7d46301a149c16e4f5561",
	            "author": {
	                "name": "Joao Jose",
	                "email": "joaojose@eu.com",
	                "date": "2015-12-01T18:57:08.000000-02:00"
	            },
	            "committer": {
	                "name": "Joao Jose",
	                "email": "joaojose@eu.com",
	                "date": ""
	            },
	            "subject": "and when he falleth, he falleth ne'er to ascend again",
	            "createdAt": "Tue Dec 1 18:57:08 2015 -0200",
	            "parent": [
	                "75239a1976f92da9b39c24cdbfae4bfb473cd0e8"
	            ]
	        }
	    ],
	    "next": "75239a1976f92da9b39c24cdbfae4bfb473cd0e8"
	}
	`
	h := testHandler{content: content}
	ts := httptest.NewServer(&h)
	defer ts.Close()
	client := Client{Endpoint: ts.URL}
	log, err := client.GetLog("repo-name", "30f221131c7d6ca50af7d46301a149c16e4f5561", "", 1)
	c.Assert(err, check.IsNil)
	c.Assert(h.url, check.Equals, "/repository/repo-name/logs?ref=30f221131c7d6ca50af7d46301a149c16e4f5561&total=1")
	c.Assert(h.method, check.Equals, "GET")
	date, err := time.Parse(GitTimeFormat, "Tue Dec 1 18:57:08 2015 -0200")
	c.Assert(err, check.IsNil)
	c.Assert(log, check.DeepEquals, Log{
		Commits: []Commit{
			{
				Ref: "30f221131c7d6ca50af7d46301a149c16e4f5561",
				Author: Author{
					Name:  "Joao Jose",
					Email: "joaojose@eu.com",
					Date:  GitTime(date),
				},
				Committer: Author{
					Name:  "Joao Jose",
					Email: "joaojose@eu.com",
					Date:  GitTime(time.Time{}),
				},
				Subject:   "and when he falleth, he falleth ne'er to ascend again",
				CreatedAt: GitTime(date),
				Parent:    []string{"75239a1976f92da9b39c24cdbfae4bfb473cd0e8"},
			},
		}, Next: "75239a1976f92da9b39c24cdbfae4bfb473cd0e8",
	})
}
