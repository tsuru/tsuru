// Copyright 2015 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package user

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/gandalf/db"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	return len(p) / 2, nil
}

type failWriter struct{}

func (failWriter) Write(p []byte) (int, error) {
	return 0, errors.New("Failed")
}

const rawKey = "ssh-dss AAAAB3NzaC1kc3MAAACBAIHfSDLpSCfIIVEJ/Is3RFMQhsCi7WZtFQeeyfi+DzVP0NGX4j/rMoQEHgXgNlOKVCJvPk5e00tukSv6iVzJPFcozArvVaoCc5jCoDi5Ef8k3Jil4Q7qNjcoRDDyqjqLcaviJEz5GrtmqAyXEIzJ447BxeEdw3Z7UrIWYcw2YyArAAAAFQD7wiOGZIoxu4XIOoeEe5aToTxN1QAAAIAZNAbJyOnNceGcgRRgBUPfY5ChX+9A29n2MGnyJ/Cxrhuh8d7B0J8UkvEBlfgQICq1UDZbC9q5NQprwD47cGwTjUZ0Z6hGpRmEEZdzsoj9T6vkLiteKH3qLo7IPVx4mV6TTF6PWQbQMUsuxjuDErwS9nhtTM4nkxYSmUbnWb6wfwAAAIB2qm/1J6Jl8bByBaMQ/ptbm4wQCvJ9Ll9u6qtKy18D4ldoXM0E9a1q49swml5CPFGyU+cgPRhEjN5oUr5psdtaY8CHa2WKuyIVH3B8UhNzqkjpdTFSpHs6tGluNVC+SQg1MVwfG2wsZUdkUGyn+6j8ZZarUfpAmbb5qJJpgMFEKQ== f@xikinbook.local"
const body = "ssh-dss AAAAB3NzaC1kc3MAAACBAIHfSDLpSCfIIVEJ/Is3RFMQhsCi7WZtFQeeyfi+DzVP0NGX4j/rMoQEHgXgNlOKVCJvPk5e00tukSv6iVzJPFcozArvVaoCc5jCoDi5Ef8k3Jil4Q7qNjcoRDDyqjqLcaviJEz5GrtmqAyXEIzJ447BxeEdw3Z7UrIWYcw2YyArAAAAFQD7wiOGZIoxu4XIOoeEe5aToTxN1QAAAIAZNAbJyOnNceGcgRRgBUPfY5ChX+9A29n2MGnyJ/Cxrhuh8d7B0J8UkvEBlfgQICq1UDZbC9q5NQprwD47cGwTjUZ0Z6hGpRmEEZdzsoj9T6vkLiteKH3qLo7IPVx4mV6TTF6PWQbQMUsuxjuDErwS9nhtTM4nkxYSmUbnWb6wfwAAAIB2qm/1J6Jl8bByBaMQ/ptbm4wQCvJ9Ll9u6qtKy18D4ldoXM0E9a1q49swml5CPFGyU+cgPRhEjN5oUr5psdtaY8CHa2WKuyIVH3B8UhNzqkjpdTFSpHs6tGluNVC+SQg1MVwfG2wsZUdkUGyn+6j8ZZarUfpAmbb5qJJpgMFEKQ==\n"
const comment = "f@xikinbook.local"
const otherKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCaNZSIEyP6FSdCX0WHDcUFTvebNbvqKiiLEiC7NTGvKrT15r2MtCDi4EPi4Ul+UyxWqb2D7FBnK1UmIcEFHd/ZCnBod2/FSplGOIbIb2UVVbqPX5Alv7IBCMyZJD14ex5cFh16zoqOsPOkOD803LMIlNvXPDDwKjY4TVOQV1JtA2tbZXvYUchqhTcKPxt5BDBZbeQkMMgUgHIEz6IueglFB3+dIZfrzlmM8CVSElKZOpucnJ5JOpGh3paSO/px2ZEcvY8WvjFdipvAWsis75GG/04F641I6XmYlo9fib/YytBXS23szqmvOqEqAopFnnGkDEo+LWI0+FXgPE8lc5BD"

func (s *S) TestNewKey(c *check.C) {
	k, err := newKey("key1", "me@tsuru.io", rawKey)
	c.Assert(err, check.IsNil)
	c.Assert(k.Name, check.Equals, "key1")
	c.Assert(k.Body, check.Equals, body)
	c.Assert(k.Comment, check.Equals, comment)
	c.Assert(k.UserName, check.Equals, "me@tsuru.io")
}

func (s *S) TestNewKeyInvalidKey(c *check.C) {
	raw := "ssh-dss ASCCDD== invalid@tsuru.io"
	k, err := newKey("key1", "me@tsuru.io", raw)
	c.Assert(k, check.IsNil)
	c.Assert(err, check.Equals, ErrInvalidKey)
}

func (s *S) TestNewKeyCreatedAtStoresCurrentTime(c *check.C) {
	k, err := newKey("key1", "me@tsuru.io", rawKey)
	c.Assert(err, check.IsNil)
	gotY, gotM, gotD := k.CreatedAt.Date()
	y, m, d := time.Now().Date()
	c.Assert(gotY, check.Equals, y)
	c.Assert(gotM, check.Equals, m)
	c.Assert(gotD, check.Equals, d)
	c.Assert(k.CreatedAt.Hour(), check.Equals, time.Now().Hour())
	c.Assert(k.CreatedAt.Minute(), check.Equals, time.Now().Minute())
}

func (s *S) TestKeyString(c *check.C) {
	k := Key{Body: "ssh-dss not-secret", Comment: "me@host"}
	c.Assert(k.String(), check.Equals, k.Body+" "+k.Comment)
}

func (s *S) TestKeyStringNewLine(c *check.C) {
	k := Key{Body: "ssh-dss not-secret\n", Comment: "me@host"}
	c.Assert(k.String(), check.Equals, "ssh-dss not-secret me@host")
}

func (s *S) TestKeyStringNoComment(c *check.C) {
	k := Key{Body: "ssh-dss not-secret"}
	c.Assert(k.String(), check.Equals, k.Body)
}

func (s *S) TestFormatKeyShouldAddSshLoginRestrictionsAtBegining(c *check.C) {
	key := Key{
		Name:     "my-key",
		Body:     "somekey\n",
		Comment:  "me@host",
		UserName: "brain",
	}
	got := key.format()
	expected := fmt.Sprintf("no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty,command=.* %s\n", &key)
	c.Assert(got, check.Matches, expected)
}

func (s *S) TestFormatKeyShouldAddCommandAfterSshRestrictions(c *check.C) {
	key := Key{
		Name:     "my-key",
		Body:     "somekey\n",
		Comment:  "me@host",
		UserName: "brain",
	}
	got := key.format()
	p, err := config.GetString("bin-path")
	c.Assert(err, check.IsNil)
	expected := fmt.Sprintf(`no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty,command="%s brain" %s`+"\n", p, &key)
	c.Assert(got, check.Equals, expected)
}

func (s *S) TestFormatKeyShouldGetCommandPathFromGandalfConf(c *check.C) {
	oldConf, err := config.GetString("bin-path")
	c.Assert(err, check.IsNil)
	config.Set("bin-path", "/foo/bar/hi.go")
	defer config.Set("bin-path", oldConf)
	key := Key{
		Name:     "my-key",
		Body:     "somekey\n",
		Comment:  "me@host",
		UserName: "dash",
	}
	got := key.format()
	expected := fmt.Sprintf(`no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty,command="/foo/bar/hi.go dash" %s`+"\n", &key)
	c.Assert(got, check.Equals, expected)
}

func (s *S) TestFormatKeyShouldAppendUserNameAsCommandParameter(c *check.C) {
	p, err := config.GetString("bin-path")
	c.Assert(err, check.IsNil)
	key := Key{
		Name:     "my-key",
		Body:     "somekey\n",
		Comment:  "me@host",
		UserName: "someuser",
	}
	got := key.format()
	expected := fmt.Sprintf(`no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty,command="%s someuser" %s`+"\n", p, &key)
	c.Assert(got, check.Equals, expected)
}

func (s *S) TestDump(c *check.C) {
	var buf bytes.Buffer
	key := Key{
		Name:     "my-key",
		Body:     "somekey\n",
		Comment:  "me@host",
		UserName: "someuser",
	}
	err := key.dump(&buf)
	c.Assert(err, check.IsNil)
	c.Assert(buf.String(), check.Equals, key.format())
}

func (s *S) TestDumpShortWrite(c *check.C) {
	key := Key{
		Name:     "my-key",
		Body:     "somekey\n",
		Comment:  "me@host",
		UserName: "someuser",
	}
	err := key.dump(shortWriter{})
	c.Assert(err, check.Equals, io.ErrShortWrite)
}

func (s *S) TestDumpWriteFailure(c *check.C) {
	key := Key{
		Name:     "my-key",
		Body:     "somekey\n",
		Comment:  "me@host",
		UserName: "someuser",
	}
	err := key.dump(failWriter{})
	c.Assert(err, check.NotNil)
}

func (s *S) TestAuthKeyUnconfigured(c *check.C) {
	home := os.Getenv("HOME")
	expected := path.Join(home, ".ssh", "authorized_keys")
	c.Assert(authKey(), check.Equals, expected)
}

func (s *S) TestAuthKeyConfig(c *check.C) {
	path := "/var/ssh/authorized_keys"
	config.Set("authorized-keys-path", path)
	defer config.Unset("authorized-keys-path")
	c.Assert(authKey(), check.Equals, path)
}

func (s *S) TestWriteKey(c *check.C) {
	key, err := newKey("my-key", "me@tsuru.io", rawKey)
	c.Assert(err, check.IsNil)
	writeKey(key)
	f, err := s.rfs.Open(authKey())
	c.Assert(err, check.IsNil)
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	c.Assert(err, check.IsNil)
	got := string(b)
	c.Assert(got, check.Equals, key.format())
}

func (s *S) TestWriteTwoKeys(c *check.C) {
	key1 := Key{
		Name:     "my-key",
		Body:     "ssh-dss mykeys-not-secret",
		Comment:  "me@machine",
		UserName: "gopher",
	}
	key2 := Key{
		Name:     "your-key",
		Body:     "ssh-dss yourkeys-not-secret",
		Comment:  "me@machine",
		UserName: "glenda",
	}
	err := writeKey(&key1)
	c.Assert(err, check.IsNil)
	err = writeKey(&key2)
	c.Assert(err, check.IsNil)
	expected := key1.format() + key2.format()
	f, err := s.rfs.Open(authKey())
	c.Assert(err, check.IsNil)
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	c.Assert(err, check.IsNil)
	got := string(b)
	c.Assert(got, check.Equals, expected)
}

func (s *S) TestAddKeyStoresKeyInTheDatabase(c *check.C) {
	err := addKey("key1", rawKey, "gopher")
	c.Assert(err, check.IsNil)
	var k Key
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Key().Find(bson.M{"name": "key1"}).One(&k)
	c.Assert(err, check.IsNil)
	defer conn.Key().Remove(bson.M{"name": "key1"})
	c.Assert(k.Name, check.Equals, "key1")
	c.Assert(k.UserName, check.Equals, "gopher")
	c.Assert(k.Comment, check.Equals, comment)
	c.Assert(k.Body, check.Equals, body)
}

func (s *S) TestAddKeyShouldSaveTheKeyInTheAuthorizedKeys(c *check.C) {
	err := addKey("key1", rawKey, "gopher")
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Key().Remove(bson.M{"name": "key1"})
	var k Key
	err = conn.Key().Find(bson.M{"name": "key1"}).One(&k)
	c.Assert(err, check.IsNil)
	f, err := s.rfs.Open(authKey())
	c.Assert(err, check.IsNil)
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	c.Assert(err, check.IsNil)
	c.Assert(string(b), check.Equals, k.format())
}

func (s *S) TestAddKeyDuplicate(c *check.C) {
	err := addKey("key1", rawKey, "gopher")
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Key().Remove(bson.M{"name": "key1"})
	err = addKey("key2", rawKey, "gopher")
	c.Assert(err, check.Equals, ErrDuplicateKey)
}

func (s *S) TestAddKeyInvalidKey(c *check.C) {
	err := addKey("key1", "something-invalid", "gopher")
	c.Assert(err, check.Equals, ErrInvalidKey)
}

func (s *S) TestUpdateKey(c *check.C) {
	err := addKey("key1", rawKey, "gopher")
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Key().Remove(bson.M{"name": "key1"})
	err = updateKey("key1", otherKey, "gopher")
	c.Assert(err, check.IsNil)
	var k Key
	err = conn.Key().Find(bson.M{"name": "key1"}).One(&k)
	c.Assert(err, check.IsNil)
	c.Assert(k.Body, check.Equals, otherKey+"\n")
	f, err := s.rfs.Open(authKey())
	c.Assert(err, check.IsNil)
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	c.Assert(err, check.IsNil)
	c.Assert(string(b), check.Equals, k.format())
}

func (s *S) TestUpdateKeyNotFound(c *check.C) {
	err := updateKey("key1", otherKey, "gopher")
	c.Assert(err, check.Equals, ErrKeyNotFound)
}

func (s *S) TestUpdateKeyInvalidKey(c *check.C) {
	err := updateKey("key1", "something-invalid", "gopher")
	c.Assert(err, check.Equals, ErrInvalidKey)
}

func (s *S) TestRemoveKeyDeletesFromDB(c *check.C) {
	err := addKey("key1", rawKey, "gopher")
	c.Assert(err, check.IsNil)
	err = removeKey("key1", "gopher")
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	count, err := conn.Key().Find(bson.M{"name": "key1"}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
}

func (s *S) TestRemoveKeyDeletesOnlyTheRightKey(c *check.C) {
	err := addKey("key1", rawKey, "gopher")
	c.Assert(err, check.IsNil)
	defer removeKey("key1", "gopher")
	err = addKey("key1", otherKey, "glenda")
	c.Assert(err, check.IsNil)
	err = removeKey("key1", "glenda")
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	count, err := conn.Key().Find(bson.M{"name": "key1", "username": "gopher"}).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 1)
}

func (s *S) TestRemoveUnknownKey(c *check.C) {
	err := removeKey("wut", "glenda")
	c.Assert(err, check.Equals, ErrKeyNotFound)
}

func (s *S) TestRemoveKeyRemovesFromAuthorizedKeys(c *check.C) {
	err := addKey("key1", rawKey, "gopher")
	c.Assert(err, check.IsNil)
	err = removeKey("key1", "gopher")
	f, err := s.rfs.Open(authKey())
	c.Assert(err, check.IsNil)
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	c.Assert(err, check.IsNil)
	got := string(b)
	c.Assert(got, check.Equals, "")
}

func (s *S) TestRemoveKeyKeepOtherKeys(c *check.C) {
	err := addKey("key1", rawKey, "gopher")
	c.Assert(err, check.IsNil)
	defer removeKey("key1", "gopher")
	err = addKey("key2", otherKey, "gopher")
	c.Assert(err, check.IsNil)
	err = removeKey("key2", "gopher")
	c.Assert(err, check.IsNil)
	var key Key
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	err = conn.Key().Find(bson.M{"name": "key1"}).One(&key)
	c.Assert(err, check.IsNil)
	f, err := s.rfs.Open(authKey())
	c.Assert(err, check.IsNil)
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	c.Assert(err, check.IsNil)
	got := string(b)
	c.Assert(got, check.Equals, key.format())
}

func (s *S) TestRemoveUserKeys(c *check.C) {
	err := addKey("key1", rawKey, "gopher")
	c.Assert(err, check.IsNil)
	defer removeKey("key1", "gopher")
	err = addKey("key1", otherKey, "glenda")
	c.Assert(err, check.IsNil)
	err = removeUserKeys("glenda")
	c.Assert(err, check.IsNil)
	var key Key
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	err = conn.Key().Find(bson.M{"name": "key1"}).One(&key)
	c.Assert(err, check.IsNil)
	f, err := s.rfs.Open(authKey())
	c.Assert(err, check.IsNil)
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	c.Assert(err, check.IsNil)
	got := string(b)
	c.Assert(got, check.Equals, key.format())
}

func (s *S) TestRemoveUserMultipleKeys(c *check.C) {
	err := addKey("key1", rawKey, "glenda")
	c.Assert(err, check.IsNil)
	err = addKey("key2", otherKey, "glenda")
	c.Assert(err, check.IsNil)
	err = removeUserKeys("glenda")
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	count, err := conn.Key().Find(nil).Count()
	c.Assert(err, check.IsNil)
	c.Assert(count, check.Equals, 0)
	f, err := s.rfs.Open(authKey())
	c.Assert(err, check.IsNil)
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	c.Assert(err, check.IsNil)
	got := string(b)
	c.Assert(got, check.Equals, "")
}

func (s *S) TestKeyListJSON(c *check.C) {
	keys := []Key{
		{Name: "key1", Body: "ssh-dss not-secret", Comment: "me@host1"},
		{Name: "key2", Body: "ssh-dss not-secret1", Comment: "me@host2"},
		{Name: "another-key", Body: "ssh-rsa not-secret", Comment: "me@work"},
	}
	expected := map[string]string{
		keys[0].Name: keys[0].String(),
		keys[1].Name: keys[1].String(),
		keys[2].Name: keys[2].String(),
	}
	var got map[string]string
	b, err := KeyList(keys).MarshalJSON()
	c.Assert(err, check.IsNil)
	err = json.Unmarshal(b, &got)
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, expected)
}

func (s *S) TestListKeys(c *check.C) {
	user := map[string]string{"_id": "glenda"}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	err = conn.User().Insert(user)
	c.Assert(err, check.IsNil)
	defer conn.User().Remove(user)
	err = addKey("key1", rawKey, "glenda")
	c.Assert(err, check.IsNil)
	err = addKey("key2", otherKey, "glenda")
	c.Assert(err, check.IsNil)
	defer removeUserKeys("glenda")
	var expected []Key
	err = conn.Key().Find(nil).All(&expected)
	c.Assert(err, check.IsNil)
	got, err := ListKeys("glenda")
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, KeyList(expected))
}

func (s *S) TestListKeysUnknownUser(c *check.C) {
	got, err := ListKeys("glenda")
	c.Assert(got, check.IsNil)
	c.Assert(err, check.Equals, ErrUserNotFound)
}

func (s *S) TestListKeysEmpty(c *check.C) {
	user := map[string]string{"_id": "gopher"}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	err = conn.User().Insert(user)
	c.Assert(err, check.IsNil)
	defer conn.User().Remove(user)
	got, err := ListKeys("gopher")
	c.Assert(err, check.IsNil)
	c.Assert(got, check.HasLen, 0)
}

func (s *S) TestListKeysFromTheUserOnly(c *check.C) {
	user := map[string]string{"_id": "gopher"}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	err = conn.User().Insert(user)
	c.Assert(err, check.IsNil)
	defer conn.User().Remove(user)
	user2 := map[string]string{"_id": "glenda"}
	err = conn.User().Insert(user2)
	c.Assert(err, check.IsNil)
	defer conn.User().Remove(user2)
	err = addKey("key1", rawKey, "glenda")
	c.Assert(err, check.IsNil)
	err = addKey("key1", otherKey, "gopher")
	c.Assert(err, check.IsNil)
	defer removeUserKeys("glenda")
	defer removeUserKeys("gopher")
	var expected []Key
	err = conn.Key().Find(bson.M{"username": "gopher"}).All(&expected)
	c.Assert(err, check.IsNil)
	got, err := ListKeys("gopher")
	c.Assert(err, check.IsNil)
	c.Assert(got, check.DeepEquals, KeyList(expected))
}
