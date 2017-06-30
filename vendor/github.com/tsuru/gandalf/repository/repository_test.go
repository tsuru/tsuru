// Copyright 2015 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"

	"github.com/tsuru/commandmocker"
	"github.com/tsuru/config"
	"github.com/tsuru/gandalf/db"
	"github.com/tsuru/gandalf/fs"
	"github.com/tsuru/gandalf/multipartzip"
	"github.com/tsuru/tsuru/fs/fstest"
	"gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct {
	tmpdir string
}

var _ = check.Suite(&S{})

func (s *S) SetUpSuite(c *check.C) {
	err := config.ReadConfigFile("../etc/gandalf.conf")
	c.Assert(err, check.IsNil)
	config.Set("database:url", "127.0.0.1:27017")
	config.Set("database:name", "gandalf_repository_tests")
}

func (s *S) TearDownSuite(c *check.C) {
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	conn.User().Database.DropDatabase()
}

func (s *S) TestTempDirLocationShouldComeFromGandalfConf(c *check.C) {
	config.Set("repository:tempDir", "/home/gandalf/temp")
	oldTempDir := tempDir
	tempDir = ""
	defer func() {
		tempDir = oldTempDir
	}()
	c.Assert(tempDirLocation(), check.Equals, "/home/gandalf/temp")
}

func (s *S) TestTempDirLocationDontResetTempDir(c *check.C) {
	config.Set("repository:tempDir", "/home/gandalf/temp")
	oldTempDir := tempDir
	tempDir = "/var/folders"
	defer func() {
		tempDir = oldTempDir
	}()
	c.Assert(tempDirLocation(), check.Equals, "/var/folders")
}

func (s *S) TestTempDirLocationWhenNotInGandalfConf(c *check.C) {
	config.Unset("repository:tempDir")
	oldTempDir := tempDir
	tempDir = ""
	defer func() {
		tempDir = oldTempDir
	}()
	c.Assert(tempDirLocation(), check.Equals, "")
}

func (s *S) TestNewShouldCreateANewRepository(c *check.C) {
	tmpdir, err := commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	users := []string{"smeagol", "saruman"}
	readOnlyUsers := []string{"gollum", "curumo"}
	r, err := New("myRepo", users, readOnlyUsers, false)
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Repository().Remove(bson.M{"_id": "myRepo"})
	c.Assert(r.Name, check.Equals, "myRepo")
	c.Assert(r.Users, check.DeepEquals, users)
	c.Assert(r.ReadOnlyUsers, check.DeepEquals, readOnlyUsers)
	c.Assert(r.IsPublic, check.Equals, false)
}

func (s *S) TestNewIntegration(c *check.C) {
	configBare, err := config.GetString("git:bare:location")
	c.Assert(err, check.IsNil)
	odlBare := bare
	bare, err = ioutil.TempDir("", "gandalf_repository_test")
	c.Assert(err, check.IsNil)
	config.Set("git:bare:location", bare)
	c.Assert(err, check.IsNil)
	defer func() {
		os.RemoveAll(bare)
		config.Set("git:bare:location", configBare)
		checkBare, configErr := config.GetString("git:bare:location")
		c.Assert(configErr, check.IsNil)
		c.Assert(checkBare, check.Equals, configBare)
		bare = odlBare
	}()
	r, err := New("the-shire", []string{"bilbo"}, []string{""}, false)
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Repository().Remove(bson.M{"_id": "the-shire"})
	barePath := barePath(r.Name)
	c.Assert(barePath, check.Equals, path.Join(bare, "the-shire.git"))
	fstat, errStat := os.Stat(path.Join(barePath, "HEAD"))
	c.Assert(errStat, check.IsNil)
	c.Assert(fstat.IsDir(), check.Equals, false)
	fstat, errStat = os.Stat(path.Join(barePath, "config"))
	c.Assert(errStat, check.IsNil)
	c.Assert(fstat.IsDir(), check.Equals, false)
	fstat, errStat = os.Stat(path.Join(barePath, "objects"))
	c.Assert(errStat, check.IsNil)
	c.Assert(fstat.IsDir(), check.Equals, true)
	fstat, errStat = os.Stat(path.Join(barePath, "refs"))
	c.Assert(errStat, check.IsNil)
	c.Assert(fstat.IsDir(), check.Equals, true)
}

func (s *S) TestNewIntegrationWithNamespace(c *check.C) {
	configBare, err := config.GetString("git:bare:location")
	c.Assert(err, check.IsNil)
	odlBare := bare
	bare, err = ioutil.TempDir("", "gandalf_repository_test")
	c.Assert(err, check.IsNil)
	config.Set("git:bare:location", bare)
	c.Assert(err, check.IsNil)
	defer func() {
		os.RemoveAll(bare)
		config.Set("git:bare:location", configBare)
		checkBare, configErr := config.GetString("git:bare:location")
		c.Assert(configErr, check.IsNil)
		c.Assert(checkBare, check.Equals, configBare)
		bare = odlBare
	}()
	r, err := New("saruman/two-towers", []string{"frodo"}, []string{""}, false)
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Repository().Remove(bson.M{"_id": "saruman/two-towers"})
	barePath := barePath(r.Name)
	c.Assert(barePath, check.Equals, path.Join(bare, "saruman/two-towers.git"))
	fstat, errStat := os.Stat(path.Join(barePath, "HEAD"))
	c.Assert(errStat, check.IsNil)
	c.Assert(fstat.IsDir(), check.Equals, false)
	fstat, errStat = os.Stat(path.Join(barePath, "config"))
	c.Assert(errStat, check.IsNil)
	c.Assert(fstat.IsDir(), check.Equals, false)
	fstat, errStat = os.Stat(path.Join(barePath, "objects"))
	c.Assert(errStat, check.IsNil)
	c.Assert(fstat.IsDir(), check.Equals, true)
	fstat, errStat = os.Stat(path.Join(barePath, "refs"))
	c.Assert(errStat, check.IsNil)
	c.Assert(fstat.IsDir(), check.Equals, true)
}

func (s *S) TestNewShouldRecordItOnDatabase(c *check.C) {
	tmpdir, err := commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	r, err := New("someRepo", []string{"smeagol"}, []string{"gollum"}, false)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Repository().Remove(bson.M{"_id": "someRepo"})
	c.Assert(err, check.IsNil)
	err = conn.Repository().Find(bson.M{"_id": "someRepo"}).One(&r)
	c.Assert(err, check.IsNil)
	c.Assert(r.Name, check.Equals, "someRepo")
	c.Assert(r.Users, check.DeepEquals, []string{"smeagol"})
	c.Assert(r.ReadOnlyUsers, check.DeepEquals, []string{"gollum"})
	c.Assert(r.IsPublic, check.Equals, false)
}

func (s *S) TestNewShouldCreateNamesakeRepositories(c *check.C) {
	tmpdir, err := commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	u1 := struct {
		Name string `bson:"_id"`
	}{Name: "melkor"}
	err = conn.User().Insert(&u1)
	c.Assert(err, check.IsNil)
	defer conn.User().RemoveId(u1.Name)
	u2 := struct {
		Name string `bson:"_id"`
	}{Name: "morgoth"}
	err = conn.User().Insert(&u2)
	c.Assert(err, check.IsNil)
	defer conn.User().RemoveId(u2.Name)
	r1, err := New("melkor/angband", []string{"nazgul"}, []string{""}, false)
	c.Assert(err, check.IsNil)
	defer conn.Repository().Remove(bson.M{"_id": "melkor/angband"})
	c.Assert(r1.Name, check.Equals, "melkor/angband")
	c.Assert(r1.IsPublic, check.Equals, false)
	r2, err := New("morgoth/angband", []string{"nazgul"}, []string{""}, false)
	c.Assert(err, check.IsNil)
	defer conn.Repository().Remove(bson.M{"_id": "morgoth/angband"})
	c.Assert(r2.Name, check.Equals, "morgoth/angband")
	c.Assert(r2.IsPublic, check.Equals, false)
}

func (s *S) TestNewPublicRepository(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "foo"}
	fs.Fsystem = rfs
	defer func() { fs.Fsystem = nil }()
	tmpdir, err := commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	r, err := New("someRepo", []string{"smeagol"}, []string{"gollum"}, true)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Repository().Remove(bson.M{"_id": "someRepo"})
	c.Assert(err, check.IsNil)
	err = conn.Repository().Find(bson.M{"_id": "someRepo"}).One(&r)
	c.Assert(err, check.IsNil)
	c.Assert(r.Name, check.Equals, "someRepo")
	c.Assert(r.Users, check.DeepEquals, []string{"smeagol"})
	c.Assert(r.ReadOnlyUsers, check.DeepEquals, []string{"gollum"})
	c.Assert(r.IsPublic, check.Equals, true)
	path := barePath("someRepo") + "/git-daemon-export-ok"
	c.Assert(rfs.HasAction("create "+path), check.Equals, true)
}

func (s *S) TestNewBreaksOnValidationError(c *check.C) {
	_, err := New("", []string{"smeagol"}, []string{""}, false)
	c.Check(err, check.NotNil)
	e, ok := err.(*InvalidRepositoryError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.message, check.Equals, "repository name is not valid")
}

func (s *S) TestNewDuplicate(c *check.C) {
	tmpdir, err := commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	users := []string{"smeagol", "saruman"}
	readOnlyUsers := []string{"gollum", "curumo"}
	r, err := New("myRepo", users, readOnlyUsers, false)
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Repository().Remove(bson.M{"_id": "myRepo"})
	err = commandmocker.Remove(tmpdir)
	c.Assert(err, check.IsNil)
	tmpdir, err = commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	r, err = New("myRepo", []string{"who"}, nil, false)
	c.Assert(err, check.Equals, ErrRepositoryAlreadyExists)
	c.Assert(r, check.IsNil)
	c.Assert(commandmocker.Ran(tmpdir), check.Equals, false)
}

func (s *S) TestRepositoryIsNotValidWithoutAName(c *check.C) {
	r := Repository{Users: []string{"gollum"}, IsPublic: true}
	v, err := r.isValid()
	c.Assert(v, check.Equals, false)
	c.Check(err, check.NotNil)
	e, ok := err.(*InvalidRepositoryError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.message, check.Equals, "repository name is not valid")
}

func (s *S) TestRepositoryIsNotValidWithInvalidName(c *check.C) {
	r := Repository{Name: "foo bar", Users: []string{"gollum"}, IsPublic: true}
	v, err := r.isValid()
	c.Assert(v, check.Equals, false)
	c.Check(err, check.NotNil)
	e, ok := err.(*InvalidRepositoryError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.message, check.Equals, "repository name is not valid")
}

func (s *S) TestRepositoryShoudBeInvalidWIthoutAnyUsers(c *check.C) {
	r := Repository{Name: "foo_bar", Users: []string{}, IsPublic: true}
	v, err := r.isValid()
	c.Assert(v, check.Equals, false)
	c.Assert(err, check.NotNil)
	e, ok := err.(*InvalidRepositoryError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.message, check.Equals, "repository should have at least one user")
}

func (s *S) TestRepositoryShoudBeInvalidWIthInvalidNamespace(c *check.C) {
	r := Repository{Name: "../repositories", Users: []string{}}
	v, err := r.isValid()
	c.Assert(v, check.Equals, false)
	c.Assert(err, check.NotNil)
	e, ok := err.(*InvalidRepositoryError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.message, check.Equals, "repository name is not valid")
	r = Repository{Name: "../../repositories", Users: []string{}}
	v, err = r.isValid()
	c.Assert(v, check.Equals, false)
	c.Assert(err, check.NotNil)
	e, ok = err.(*InvalidRepositoryError)
	c.Assert(ok, check.Equals, true)
	c.Assert(e.message, check.Equals, "repository name is not valid")
}

func (s *S) TestRepositoryAcceptsValidNamespaces(c *check.C) {
	r := Repository{Name: "_.mallory/foo_bar", Users: []string{"alice", "bob"}}
	v, err := r.isValid()
	c.Assert(v, check.Equals, true)
	c.Assert(err, check.IsNil)
	r = Repository{Name: "_git/foo_bar", Users: []string{"alice", "bob"}}
	v, err = r.isValid()
	c.Assert(v, check.Equals, true)
	c.Assert(err, check.IsNil)
	r = Repository{Name: "time-home_rc2+beta@globoi.com/foo_bar", Users: []string{"you", "me"}}
	v, err = r.isValid()
	c.Assert(v, check.Equals, true)
	c.Assert(err, check.IsNil)
}

func (s *S) TestRepositoryShouldBeValidWithoutIsPublic(c *check.C) {
	r := Repository{Name: "someName", Users: []string{"smeagol"}}
	v, _ := r.isValid()
	c.Assert(v, check.Equals, true)
}

func (s *S) TestNewShouldCreateNewGitBareRepository(c *check.C) {
	tmpdir, err := commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	_, err = New("myRepo", []string{"pumpkin"}, []string{""}, true)
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Repository().Remove(bson.M{"_id": "myRepo"})
	c.Assert(commandmocker.Ran(tmpdir), check.Equals, true)
}

func (s *S) TestNewShouldNotStoreRepoInDbWhenBareCreationFails(c *check.C) {
	dir, err := commandmocker.Error("git", "", 1)
	c.Check(err, check.IsNil)
	defer commandmocker.Remove(dir)
	r, err := New("myRepo", []string{"pumpkin"}, []string{""}, true)
	c.Check(err, check.NotNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Repository().Find(bson.M{"_id": r.Name}).One(&r)
	c.Assert(err, check.ErrorMatches, "^not found$")
}

func (s *S) TestRemoveShouldRemoveBareRepositoryFromFileSystem(c *check.C) {
	tmpdir, err := commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	rfs := &fstest.RecordingFs{FileContent: "foo"}
	fs.Fsystem = rfs
	defer func() { fs.Fsystem = nil }()
	r, err := New("myRepo", []string{"pumpkin"}, []string{""}, false)
	c.Assert(err, check.IsNil)
	err = Remove(r.Name)
	c.Assert(err, check.IsNil)
	action := "removeall " + path.Join(bareLocation(), "myRepo.git")
	c.Assert(rfs.HasAction(action), check.Equals, true)
}

func (s *S) TestRemoveShouldRemoveRepositoryFromDatabase(c *check.C) {
	tmpdir, err := commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	rfs := &fstest.RecordingFs{FileContent: "foo"}
	fs.Fsystem = rfs
	defer func() { fs.Fsystem = nil }()
	r, err := New("myRepo", []string{"pumpkin"}, []string{""}, false)
	c.Assert(err, check.IsNil)
	err = Remove(r.Name)
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Repository().Find(bson.M{"_id": r.Name}).One(&r)
	c.Assert(err, check.ErrorMatches, "^not found$")
}

func (s *S) TestRemoveNotFound(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "foo"}
	fs.Fsystem = rfs
	defer func() { fs.Fsystem = nil }()
	r := &Repository{Name: "fooBar"}
	err := Remove(r.Name)
	c.Assert(err, check.Equals, ErrRepositoryNotFound)
}

func (s *S) TestUpdate(c *check.C) {
	tmpdir, err := commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	r, err := New("freedom", []string{"c"}, []string{"d"}, false)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Repository().RemoveId(r.Name)
	expected := Repository{
		Name:          "freedom",
		Users:         []string{"a", "b"},
		ReadOnlyUsers: []string{"c", "d"},
		IsPublic:      true,
	}
	err = Update(r.Name, expected)
	c.Assert(err, check.IsNil)
	repo, err := Get("freedom")
	c.Assert(err, check.IsNil)
	c.Assert(repo, check.DeepEquals, expected)
}

func (s *S) TestUpdateWithRenaming(c *check.C) {
	tmpdir, err := commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	r, err := New("freedom", []string{"c"}, []string{"d"}, false)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Repository().RemoveId(r.Name)
	rfs := &fstest.RecordingFs{}
	fs.Fsystem = rfs
	defer func() { fs.Fsystem = nil }()
	expected := Repository{
		Name:          "freedom2",
		Users:         []string{"a", "b"},
		ReadOnlyUsers: []string{"c", "d"},
		IsPublic:      true,
	}
	err = Update(r.Name, expected)
	c.Assert(err, check.IsNil)
	repo, err := Get("freedom")
	c.Assert(err, check.NotNil)
	repo, err = Get("freedom2")
	c.Assert(err, check.IsNil)
	c.Assert(repo, check.DeepEquals, expected)
	oldPath := path.Join(bareLocation(), "freedom.git")
	newPath := path.Join(bareLocation(), "freedom2.git")
	action := fmt.Sprintf("rename %s %s", oldPath, newPath)
	c.Assert(rfs.HasAction(action), check.Equals, true)
}

func (s *S) TestUpdateErrsWithAlreadyExists(c *check.C) {
	tmpdir, err := commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	r1, err := New("freedom", []string{"free"}, []string{}, false)
	c.Assert(err, check.IsNil)
	r2, err := New("subjection", []string{"subdued"}, []string{}, false)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Repository().RemoveId(r1.Name)
	defer conn.Repository().RemoveId(r2.Name)
	update := Repository{
		Name: "subjection",
	}
	err = Update(r1.Name, update)
	c.Assert(mgo.IsDup(err), check.Equals, true)
}

func (s *S) TestUpdateErrsWhenNotFound(c *check.C) {
	update := Repository{}
	err := Update("nonexistent", update)
	c.Assert(err, check.Equals, ErrRepositoryNotFound)
}

func (s *S) TestReadOnlyURL(c *check.C) {
	host, err := config.GetString("host")
	c.Assert(err, check.IsNil)
	remote := (&Repository{Name: "lol"}).ReadOnlyURL()
	c.Assert(remote, check.Equals, fmt.Sprintf("git://%s/lol.git", host))
}

func (s *S) TestReadOnlyURLWithNamespace(c *check.C) {
	host, err := config.GetString("host")
	c.Assert(err, check.IsNil)
	remote := (&Repository{Name: "olo/lol"}).ReadOnlyURL()
	c.Assert(remote, check.Equals, fmt.Sprintf("git://%s/olo/lol.git", host))
}

func (s *S) TestReadOnlyURLWithSSH(c *check.C) {
	config.Set("git:ssh:use", true)
	defer config.Unset("git:ssh:use")
	host, err := config.GetString("host")
	c.Assert(err, check.IsNil)
	remote := (&Repository{Name: "lol"}).ReadOnlyURL()
	c.Assert(remote, check.Equals, fmt.Sprintf("ssh://git@%s/lol.git", host))
}

func (s *S) TestReadOnlyURLWithSSHAndPort(c *check.C) {
	config.Set("git:ssh:use", true)
	defer config.Unset("git:ssh:use")
	config.Set("git:ssh:port", "49022")
	defer config.Unset("git:ssh:port")
	host, err := config.GetString("host")
	c.Assert(err, check.IsNil)
	remote := (&Repository{Name: "lol"}).ReadOnlyURL()
	c.Assert(remote, check.Equals, fmt.Sprintf("ssh://git@%s:49022/lol.git", host))
}

func (s *S) TestReadOnlyURLWithReadOnlyHost(c *check.C) {
	config.Set("readonly-host", "something-private")
	defer config.Unset("readonly-host")
	remote := (&Repository{Name: "lol"}).ReadOnlyURL()
	c.Assert(remote, check.Equals, "git://something-private/lol.git")
}

func (s *S) TestReadWriteURLWithSSH(c *check.C) {
	config.Set("git:ssh:use", true)
	defer config.Unset("git:ssh:use")
	uid, err := config.GetString("uid")
	c.Assert(err, check.IsNil)
	host, err := config.GetString("host")
	c.Assert(err, check.IsNil)
	remote := (&Repository{Name: "lol"}).ReadWriteURL()
	expected := fmt.Sprintf("ssh://%s@%s/lol.git", uid, host)
	c.Assert(remote, check.Equals, expected)
}

func (s *S) TestReadWriteURLWithNamespaceAndSSH(c *check.C) {
	config.Set("git:ssh:use", true)
	defer config.Unset("git:ssh:use")
	uid, err := config.GetString("uid")
	c.Assert(err, check.IsNil)
	host, err := config.GetString("host")
	c.Assert(err, check.IsNil)
	remote := (&Repository{Name: "olo/lol"}).ReadWriteURL()
	expected := fmt.Sprintf("ssh://%s@%s/olo/lol.git", uid, host)
	c.Assert(remote, check.Equals, expected)
}

func (s *S) TestReadWriteURLWithSSHAndPort(c *check.C) {
	config.Set("git:ssh:use", true)
	defer config.Unset("git:ssh:use")
	config.Set("git:ssh:port", "49022")
	defer config.Unset("git:ssh:port")
	uid, err := config.GetString("uid")
	c.Assert(err, check.IsNil)
	host, err := config.GetString("host")
	c.Assert(err, check.IsNil)
	remote := (&Repository{Name: "lol"}).ReadWriteURL()
	expected := fmt.Sprintf("ssh://%s@%s:49022/lol.git", uid, host)
	c.Assert(remote, check.Equals, expected)
}

func (s *S) TestReadWriteURL(c *check.C) {
	host, err := config.GetString("host")
	c.Assert(err, check.IsNil)
	remote := (&Repository{Name: "lol"}).ReadWriteURL()
	c.Assert(remote, check.Equals, fmt.Sprintf("git@%s:lol.git", host))
}

func (s *S) TestReadWriteURLUseUidFromConfigFile(c *check.C) {
	uid, err := config.GetString("uid")
	c.Assert(err, check.IsNil)
	host, err := config.GetString("host")
	c.Assert(err, check.IsNil)
	config.Set("uid", "test")
	defer config.Set("uid", uid)
	remote := (&Repository{Name: "f#"}).ReadWriteURL()
	c.Assert(remote, check.Equals, fmt.Sprintf("test@%s:f#.git", host))
}

func (s *S) TestGrantAccessShouldAddUserToListOfRepositories(c *check.C) {
	tmpdir, err := commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	r, err := New("proj1", []string{"someuser"}, []string{"otheruser"}, true)
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Repository().RemoveId(r.Name)
	r2, err := New("proj2", []string{"otheruser"}, []string{"someuser"}, true)
	c.Assert(err, check.IsNil)
	defer conn.Repository().RemoveId(r2.Name)
	u := struct {
		Name string `bson:"_id"`
	}{Name: "lolcat"}
	err = conn.User().Insert(&u)
	c.Assert(err, check.IsNil)
	defer conn.User().RemoveId(u.Name)
	err = GrantAccess([]string{r.Name, r2.Name}, []string{u.Name}, false)
	c.Assert(err, check.IsNil)
	err = conn.Repository().FindId(r.Name).One(&r)
	c.Assert(err, check.IsNil)
	err = conn.Repository().FindId(r2.Name).One(&r2)
	c.Assert(err, check.IsNil)
	c.Assert(r.Users, check.DeepEquals, []string{"someuser", u.Name})
	c.Assert(r2.Users, check.DeepEquals, []string{"otheruser", u.Name})
	c.Assert(r.ReadOnlyUsers, check.DeepEquals, []string{"otheruser"})
	c.Assert(r2.ReadOnlyUsers, check.DeepEquals, []string{"someuser"})
}

func (s *S) TestGrantReadOnlyAccessShouldAddUserToListOfRepositories(c *check.C) {
	tmpdir, err := commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	r, err := New("proj1", []string{"someuser"}, []string{"otheruser"}, true)
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Repository().RemoveId(r.Name)
	r2, err := New("proj2", []string{"otheruser"}, []string{"someuser"}, true)
	c.Assert(err, check.IsNil)
	defer conn.Repository().RemoveId(r2.Name)
	u := struct {
		Name string `bson:"_id"`
	}{Name: "lolcat"}
	err = conn.User().Insert(&u)
	c.Assert(err, check.IsNil)
	defer conn.User().RemoveId(u.Name)
	err = GrantAccess([]string{r.Name, r2.Name}, []string{u.Name}, true)
	c.Assert(err, check.IsNil)
	err = conn.Repository().FindId(r.Name).One(&r)
	c.Assert(err, check.IsNil)
	err = conn.Repository().FindId(r2.Name).One(&r2)
	c.Assert(err, check.IsNil)
	c.Assert(r.Users, check.DeepEquals, []string{"someuser"})
	c.Assert(r2.Users, check.DeepEquals, []string{"otheruser"})
	c.Assert(r.ReadOnlyUsers, check.DeepEquals, []string{"otheruser", u.Name})
	c.Assert(r2.ReadOnlyUsers, check.DeepEquals, []string{"someuser", u.Name})
}

func (s *S) TestGrantAccessShouldAddFirstUserIntoRepositoryDocument(c *check.C) {
	r := Repository{Name: "proj1"}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Repository().Insert(&r)
	c.Assert(err, check.IsNil)
	defer conn.Repository().RemoveId(r.Name)
	r2 := Repository{Name: "proj2"}
	err = conn.Repository().Insert(&r2)
	c.Assert(err, check.IsNil)
	defer conn.Repository().RemoveId(r2.Name)
	err = GrantAccess([]string{r.Name, r2.Name}, []string{"Umi"}, false)
	c.Assert(err, check.IsNil)
	err = conn.Repository().FindId(r.Name).One(&r)
	c.Assert(err, check.IsNil)
	err = conn.Repository().FindId(r.Name).One(&r2)
	c.Assert(err, check.IsNil)
	c.Assert(r.Users, check.DeepEquals, []string{"Umi"})
	c.Assert(r2.Users, check.DeepEquals, []string{"Umi"})
}

func (s *S) TestGrantAccessShouldSkipDuplicatedUsers(c *check.C) {
	r := Repository{Name: "proj1", Users: []string{"umi", "luke", "pade"}}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Repository().Insert(&r)
	c.Assert(err, check.IsNil)
	defer conn.Repository().RemoveId(r.Name)
	err = GrantAccess([]string{r.Name}, []string{"pade"}, false)
	c.Assert(err, check.IsNil)
	err = conn.Repository().FindId(r.Name).One(&r)
	c.Assert(err, check.IsNil)
	c.Assert(r.Users, check.DeepEquals, []string{"umi", "luke", "pade"})
}

func (s *S) TestGrantAccessNotFound(c *check.C) {
	err := GrantAccess([]string{"super-repo"}, []string{"someuser"}, false)
	c.Assert(err, check.Equals, ErrRepositoryNotFound)
}

func (s *S) TestRevokeAccessShouldRemoveUserFromAllRepositories(c *check.C) {
	tmpdir, err := commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	r, err := New("proj1", []string{"someuser", "umi"}, []string{"otheruser"}, true)
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Repository().RemoveId(r.Name)
	r2, err := New("proj2", []string{"otheruser", "umi"}, []string{"someuser"}, true)
	c.Assert(err, check.IsNil)
	defer conn.Repository().RemoveId(r2.Name)
	err = RevokeAccess([]string{r.Name, r2.Name}, []string{"umi"}, false)
	c.Assert(err, check.IsNil)
	err = conn.Repository().FindId(r.Name).One(&r)
	c.Assert(err, check.IsNil)
	err = conn.Repository().FindId(r2.Name).One(&r2)
	c.Assert(err, check.IsNil)
	c.Assert(r.Users, check.DeepEquals, []string{"someuser"})
	c.Assert(r2.Users, check.DeepEquals, []string{"otheruser"})
	c.Assert(r.ReadOnlyUsers, check.DeepEquals, []string{"otheruser"})
	c.Assert(r2.ReadOnlyUsers, check.DeepEquals, []string{"someuser"})
}

func (s *S) TestRevokeReadOnlyAccessShouldRemoveUserFromAllRepositories(c *check.C) {
	tmpdir, err := commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	r, err := New("proj1", []string{"someuser"}, []string{"otheruser", "umi"}, true)
	c.Assert(err, check.IsNil)
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	defer conn.Repository().RemoveId(r.Name)
	r2, err := New("proj2", []string{"otheruser"}, []string{"someuser", "umi"}, true)
	c.Assert(err, check.IsNil)
	defer conn.Repository().RemoveId(r2.Name)
	err = RevokeAccess([]string{r.Name, r2.Name}, []string{"umi"}, true)
	c.Assert(err, check.IsNil)
	err = conn.Repository().FindId(r.Name).One(&r)
	c.Assert(err, check.IsNil)
	err = conn.Repository().FindId(r2.Name).One(&r2)
	c.Assert(err, check.IsNil)
	c.Assert(r.Users, check.DeepEquals, []string{"someuser"})
	c.Assert(r2.Users, check.DeepEquals, []string{"otheruser"})
	c.Assert(r.ReadOnlyUsers, check.DeepEquals, []string{"otheruser"})
	c.Assert(r2.ReadOnlyUsers, check.DeepEquals, []string{"someuser"})
}

func (s *S) TestRevokeAccessNotFound(c *check.C) {
	err := RevokeAccess([]string{"super-repo"}, []string{"someuser"}, false)
	c.Assert(err, check.Equals, ErrRepositoryNotFound)
}

func (s *S) TestGet(c *check.C) {
	repo := Repository{Name: "somerepo", Users: []string{}, ReadOnlyUsers: []string{}}
	conn, err := db.Conn()
	c.Assert(err, check.IsNil)
	defer conn.Close()
	err = conn.Repository().Insert(repo)
	c.Assert(err, check.IsNil)
	r, err := Get("somerepo")
	c.Assert(err, check.IsNil)
	c.Assert(r, check.DeepEquals, repo)
}

func (s *S) TestGetNotFound(c *check.C) {
	_, err := Get("unknown-repository")
	c.Assert(err, check.Equals, ErrRepositoryNotFound)
}

func (s *S) TestMarshalJSON(c *check.C) {
	repo := Repository{Name: "somerepo", Users: []string{}}
	expected := map[string]interface{}{
		"name":    repo.Name,
		"public":  repo.IsPublic,
		"ssh_url": repo.ReadWriteURL(),
		"git_url": repo.ReadOnlyURL(),
	}
	data, err := json.Marshal(&repo)
	c.Assert(err, check.IsNil)
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	c.Assert(err, check.IsNil)
	c.Assert(result, check.DeepEquals, expected)
}

func (s *S) TestGetFileContentsWhenContentsAvailable(c *check.C) {
	expected := []byte("something")
	Retriever = &MockContentRetriever{
		ResultContents: expected,
	}
	defer func() {
		Retriever = nil
	}()
	contents, err := GetFileContents("repo", "ref", "path")
	c.Assert(err, check.IsNil)
	c.Assert(string(contents), check.Equals, string(expected))
}

func (s *S) TestGetFileContentsWhenGitNotFound(c *check.C) {
	lookpathError := fmt.Errorf("mock lookpath error")
	Retriever = &MockContentRetriever{
		LookPathError: lookpathError,
	}
	defer func() {
		Retriever = nil
	}()
	_, err := GetFileContents("repo", "ref", "path")
	c.Assert(err.Error(), check.Equals, "mock lookpath error")
}

func (s *S) TestGetFileContentsWhenCommandFails(c *check.C) {
	outputError := fmt.Errorf("mock output error")
	Retriever = &MockContentRetriever{
		OutputError: outputError,
	}
	defer func() {
		Retriever = nil
	}()
	_, err := GetFileContents("repo", "ref", "path")
	c.Assert(err.Error(), check.Equals, "mock output error")
}

func (s *S) TestGetArchive(c *check.C) {
	expected := []byte("something")
	Retriever = &MockContentRetriever{
		ResultContents: expected,
	}
	defer func() {
		Retriever = nil
	}()
	contents, err := GetArchive("repo", "ref", Zip)
	c.Assert(err, check.IsNil)
	c.Assert(string(contents), check.Equals, string(expected))
}

func (s *S) TestGetArchiveWhenGitNotFound(c *check.C) {
	lookpathError := fmt.Errorf("mock lookpath error")
	Retriever = &MockContentRetriever{
		LookPathError: lookpathError,
	}
	defer func() {
		Retriever = nil
	}()
	_, err := GetArchive("repo", "ref", Zip)
	c.Assert(err.Error(), check.Equals, "mock lookpath error")
}

func (s *S) TestGetArchiveWhenCommandFails(c *check.C) {
	outputError := fmt.Errorf("mock output error")
	Retriever = &MockContentRetriever{
		OutputError: outputError,
	}
	defer func() {
		Retriever = nil
	}()
	_, err := GetArchive("repo", "ref", Zip)
	c.Assert(err.Error(), check.Equals, "mock output error")
}

func (s *S) TestGetFileContentIntegration(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	contents, err := GetFileContents(repo, "master", file)
	c.Assert(err, check.IsNil)
	c.Assert(string(contents), check.Equals, content)
}

func (s *S) TestGetFileContentIntegrationEmptyContent(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := ""
	cleanUp, errCreate := CreateEmptyTestRepository(bare, repo)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	err := CreateEmptyFile(bare, repo, file)
	c.Assert(err, check.IsNil)
	testPath := path.Join(bare, repo+".git")
	err = MakeCommit(testPath, "empty file content")
	c.Assert(err, check.IsNil)
	contents, err := GetFileContents(repo, "master", file)
	c.Assert(err, check.IsNil)
	c.Assert(string(contents), check.Equals, content)
}

func (s *S) TestGetFileContentWhenRefIsInvalid(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	_, err := GetFileContents(repo, "MuchMissing", file)
	c.Assert(err, check.ErrorMatches, "^Error when trying to obtain file README on ref MuchMissing of repository gandalf-test-repo \\(exit status 128\\)\\.$")
}

func (s *S) TestGetFileContentWhenFileIsInvalid(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	_, err := GetFileContents(repo, "master", "Such file")
	c.Assert(err, check.ErrorMatches, "^Error when trying to obtain file Such file on ref master of repository gandalf-test-repo \\(exit status 128\\)\\.$")
}

func (s *S) TestGetTreeIntegration(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content, "such", "folder", "much", "magic")
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	tree, err := GetTree(repo, "master", "much/README")
	c.Assert(err, check.IsNil)
	c.Assert(tree[0]["path"], check.Equals, "much/README")
	c.Assert(tree[0]["rawPath"], check.Equals, "much/README")
}

func (s *S) TestGetTreeIntegrationEmptyContent(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := ""
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content, "such", "folder", "much", "magic")
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	tree, err := GetTree(repo, "master", "much/README")
	c.Assert(err, check.IsNil)
	c.Assert(tree[0]["path"], check.Equals, "much/README")
	c.Assert(tree[0]["rawPath"], check.Equals, "much/README")
}

func (s *S) TestGetTreeIntegrationWithEscapedFileName(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "such\tREADME"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content, "such", "folder", "much", "magic")
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	tree, err := GetTree(repo, "master", "much/such\tREADME")
	c.Assert(err, check.IsNil)
	c.Assert(tree[0]["path"], check.Equals, "much/such\\tREADME")
	c.Assert(tree[0]["rawPath"], check.Equals, "\"much/such\\tREADME\"")
}

func (s *S) TestGetTreeIntegrationWithFileNameWithSpace(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "much README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content, "such", "folder", "much", "magic")
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	tree, err := GetTree(repo, "master", "much/much README")
	c.Assert(err, check.IsNil)
	c.Assert(tree[0]["path"], check.Equals, "much/much README")
	c.Assert(tree[0]["rawPath"], check.Equals, "much/much README")
}

func (s *S) TestGetArchiveIntegrationWhenZip(c *check.C) {
	expected := make(map[string]string)
	expected["gandalf-test-repo-master/README"] = "much WOW"
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	zipContents, err := GetArchive(repo, "master", Zip)
	reader := bytes.NewReader(zipContents)
	zipReader, err := zip.NewReader(reader, int64(len(zipContents)))
	c.Assert(err, check.IsNil)
	for _, f := range zipReader.File {
		rc, err := f.Open()
		c.Assert(err, check.IsNil)
		defer rc.Close()
		contents, err := ioutil.ReadAll(rc)
		c.Assert(err, check.IsNil)
		c.Assert(string(contents), check.Equals, expected[f.Name])
	}
}

func (s *S) TestGetArchiveIntegrationWhenTar(c *check.C) {
	expected := make(map[string]string)
	expected["gandalf-test-repo-master/README"] = "much WOW"
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	tarContents, err := GetArchive(repo, "master", Tar)
	c.Assert(err, check.IsNil)
	reader := bytes.NewReader(tarContents)
	tarReader := tar.NewReader(reader)
	c.Assert(err, check.IsNil)
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			// end of tar archive
			break
		}
		c.Assert(err, check.IsNil)
		path := hdr.Name
		_, ok := expected[path]
		if !ok {
			continue
		}
		buffer := new(bytes.Buffer)
		_, err = io.Copy(buffer, tarReader)
		c.Assert(err, check.IsNil)
		c.Assert(buffer.String(), check.Equals, expected[path])
	}
}

func (s *S) TestGetArchiveIntegrationWhenInvalidFormat(c *check.C) {
	expected := make(map[string]string)
	expected["gandalf-test-repo-master/README"] = "much WOW"
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	zipContents, err := GetArchive(repo, "master", 99)
	reader := bytes.NewReader(zipContents)
	zipReader, err := zip.NewReader(reader, int64(len(zipContents)))
	c.Assert(err, check.IsNil)
	for _, f := range zipReader.File {
		//fmt.Printf("Contents of %s:\n", f.Name)
		rc, err := f.Open()
		c.Assert(err, check.IsNil)
		defer rc.Close()
		contents, err := ioutil.ReadAll(rc)
		c.Assert(err, check.IsNil)
		c.Assert(string(contents), check.Equals, expected[f.Name])
	}
}

func (s *S) TestGetArchiveIntegrationWhenInvalidRepo(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	_, err := GetArchive("invalid-repo", "master", Zip)
	c.Assert(err.Error(), check.Equals, "Error when trying to obtain archive for ref master of repository invalid-repo (Repository does not exist).")
}

func (s *S) TestGetTreeIntegrationWithMissingFile(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "very WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	tree, err := GetTree(repo, "master", "very missing")
	c.Assert(err, check.IsNil)
	c.Assert(tree, check.HasLen, 0)
}

func (s *S) TestGetTreeIntegrationWithInvalidRef(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "very WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	_, err := GetTree(repo, "VeryInvalid", "very missing")
	c.Assert(err, check.ErrorMatches, "^Error when trying to obtain tree very missing on ref VeryInvalid of repository gandalf-test-repo \\(exit status 128\\)\\.$")
}

func (s *S) TestGetBranchesIntegration(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "will bark"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	errCreateBranches := CreateBranchesOnTestRepository(bare, repo, "doge_bites", "doge_barks")
	c.Assert(errCreateBranches, check.IsNil)
	branches, err := GetBranches(repo)
	c.Assert(err, check.IsNil)
	c.Assert(branches, check.HasLen, 3)
	c.Assert(branches[0].Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(branches[0].Name, check.Equals, "doge_barks")
	c.Assert(branches[0].Committer.Name, check.Equals, "doge")
	c.Assert(branches[0].Committer.Email, check.Equals, "<much@email.com>")
	c.Assert(branches[0].Author.Name, check.Equals, "doge")
	c.Assert(branches[0].Author.Email, check.Equals, "<much@email.com>")
	c.Assert(branches[0].Subject, check.Equals, "will bark")
	c.Assert(branches[0].CreatedAt, check.Equals, branches[0].Author.Date)
	c.Assert(branches[0].Links.ZipArchive, check.Equals, GetArchiveUrl(repo, "doge_barks", "zip"))
	c.Assert(branches[0].Links.TarArchive, check.Equals, GetArchiveUrl(repo, "doge_barks", "tar.gz"))
	c.Assert(branches[1].Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(branches[1].Name, check.Equals, "doge_bites")
	c.Assert(branches[1].Committer.Name, check.Equals, "doge")
	c.Assert(branches[1].Committer.Email, check.Equals, "<much@email.com>")
	c.Assert(branches[1].Author.Name, check.Equals, "doge")
	c.Assert(branches[1].Author.Email, check.Equals, "<much@email.com>")
	c.Assert(branches[1].Subject, check.Equals, "will bark")
	c.Assert(branches[1].CreatedAt, check.Equals, branches[1].Author.Date)
	c.Assert(branches[1].Links.ZipArchive, check.Equals, GetArchiveUrl(repo, "doge_bites", "zip"))
	c.Assert(branches[1].Links.TarArchive, check.Equals, GetArchiveUrl(repo, "doge_bites", "tar.gz"))
	c.Assert(branches[2].Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(branches[2].Name, check.Equals, "master")
	c.Assert(branches[2].Committer.Name, check.Equals, "doge")
	c.Assert(branches[2].Committer.Email, check.Equals, "<much@email.com>")
	c.Assert(branches[2].Author.Name, check.Equals, "doge")
	c.Assert(branches[2].Author.Email, check.Equals, "<much@email.com>")
	c.Assert(branches[2].Subject, check.Equals, "will bark")
	c.Assert(branches[2].CreatedAt, check.Equals, branches[2].Author.Date)
	c.Assert(branches[2].Links.ZipArchive, check.Equals, GetArchiveUrl(repo, "master", "zip"))
	c.Assert(branches[2].Links.TarArchive, check.Equals, GetArchiveUrl(repo, "master", "tar.gz"))
}

func (s *S) TestGetForEachRefIntegrationWithSubjectEmpty(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := ""
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	errCreateBranches := CreateBranchesOnTestRepository(bare, repo, "doge_howls")
	c.Assert(errCreateBranches, check.IsNil)
	refs, err := GetForEachRef(repo, "refs/")
	c.Assert(err, check.IsNil)
	c.Assert(refs, check.HasLen, 2)
	c.Assert(refs[0].Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(refs[0].Name, check.Equals, "doge_howls")
	c.Assert(refs[0].Committer.Name, check.Equals, "doge")
	c.Assert(refs[0].Committer.Email, check.Equals, "<much@email.com>")
	c.Assert(refs[0].Author.Name, check.Equals, "doge")
	c.Assert(refs[0].Author.Email, check.Equals, "<much@email.com>")
	c.Assert(refs[0].Subject, check.Equals, "")
	c.Assert(refs[0].CreatedAt, check.Equals, refs[0].Author.Date)
	c.Assert(refs[1].Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(refs[1].Name, check.Equals, "master")
	c.Assert(refs[1].Committer.Name, check.Equals, "doge")
	c.Assert(refs[1].Committer.Email, check.Equals, "<much@email.com>")
	c.Assert(refs[1].Author.Name, check.Equals, "doge")
	c.Assert(refs[1].Author.Email, check.Equals, "<much@email.com>")
	c.Assert(refs[1].Subject, check.Equals, "")
	c.Assert(refs[1].CreatedAt, check.Equals, refs[1].Author.Date)
}

func (s *S) TestGetForEachRefIntegrationWithSubjectTabbed(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "will\tbark"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	errCreateBranches := CreateBranchesOnTestRepository(bare, repo, "doge_howls")
	c.Assert(errCreateBranches, check.IsNil)
	refs, err := GetForEachRef(repo, "refs/")
	c.Assert(err, check.IsNil)
	c.Assert(refs, check.HasLen, 2)
	c.Assert(refs[0].Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(refs[0].Name, check.Equals, "doge_howls")
	c.Assert(refs[0].Committer.Name, check.Equals, "doge")
	c.Assert(refs[0].Committer.Email, check.Equals, "<much@email.com>")
	c.Assert(refs[0].Author.Name, check.Equals, "doge")
	c.Assert(refs[0].Author.Email, check.Equals, "<much@email.com>")
	c.Assert(refs[0].Subject, check.Equals, "will\tbark")
	c.Assert(refs[0].CreatedAt, check.Equals, refs[0].Author.Date)
	c.Assert(refs[1].Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(refs[1].Name, check.Equals, "master")
	c.Assert(refs[1].Committer.Name, check.Equals, "doge")
	c.Assert(refs[1].Committer.Email, check.Equals, "<much@email.com>")
	c.Assert(refs[1].Author.Name, check.Equals, "doge")
	c.Assert(refs[1].Author.Email, check.Equals, "<much@email.com>")
	c.Assert(refs[1].Subject, check.Equals, "will\tbark")
	c.Assert(refs[1].CreatedAt, check.Equals, refs[1].Author.Date)
}

func (s *S) TestGetForEachRefIntegrationWhenPatternEmpty(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	refs, err := GetForEachRef("gandalf-test-repo", "")
	c.Assert(err, check.IsNil)
	c.Assert(refs, check.HasLen, 1)
	c.Assert(refs[0], check.FitsTypeOf, Ref{})
	c.Assert(refs[0].Name, check.Equals, "master")
}

func (s *S) TestGetForEachRefIntegrationWhenPatternNonExistent(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	refs, err := GetForEachRef("gandalf-test-repo", "non_existent_pattern")
	c.Assert(err, check.IsNil)
	c.Assert(refs, check.HasLen, 0)
}

func (s *S) TestGetForEachRefIntegrationWhenInvalidRepo(c *check.C) {
	_, err := GetForEachRef("invalid-repo", "refs/")
	c.Assert(err.Error(), check.Equals, "Error when trying to obtain the refs of repository invalid-repo (Repository does not exist).")
}

func (s *S) TestGetForEachRefIntegrationWhenPatternSpaced(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	errCreateBranches := CreateBranchesOnTestRepository(bare, repo, "doge_howls")
	c.Assert(errCreateBranches, check.IsNil)
	refs, err := GetForEachRef("gandalf-test-repo", "much bark")
	c.Assert(err, check.IsNil)
	c.Assert(refs, check.HasLen, 0)
}

func (s *S) TestGetForEachRefIntegrationWhenPatternInvalid(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	_, err := GetForEachRef("gandalf-test-repo", "--format")
	c.Assert(err.Error(), check.Equals, "Error when trying to obtain the refs of repository gandalf-test-repo (exit status 129).")
}

func (s *S) TestGetForEachRefWithSomeEmptyFields(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	tmpdir, err := commandmocker.Add("git", "ec083c5f40be15e2bf5a84efe83d8f4723a6dcc0\tmaster\t\t\t\t\t\t\t\t\t\t")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	refs, err := GetForEachRef(repo, "")
	c.Assert(err, check.IsNil)
	c.Assert(refs, check.HasLen, 1)
}

func (s *S) TestGetForEachRefOutputInvalid(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	tmpdir, err := commandmocker.Add("git", "-")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	refs, err := GetForEachRef(repo, "")
	c.Assert(err, check.IsNil)
	c.Assert(refs, check.HasLen, 0)
}

func (s *S) TestGetForEachRefOutputEmpty(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	tmpdir, err := commandmocker.Add("git", "\n")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	refs, err := GetForEachRef(repo, "")
	c.Assert(err, check.IsNil)
	c.Assert(refs, check.HasLen, 0)
}

func (s *S) TestGetDiffIntegration(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "Just a regular readme."
	object1 := "You should read this README"
	object2 := "Seriously, read this file!"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	errCreateCommit := CreateCommit(bare, repo, file, object1)
	c.Assert(errCreateCommit, check.IsNil)
	firstHashCommit, err := GetLastHashCommit(bare, repo)
	c.Assert(err, check.IsNil)
	errCreateCommit = CreateCommit(bare, repo, file, object2)
	c.Assert(errCreateCommit, check.IsNil)
	secondHashCommit, err := GetLastHashCommit(bare, repo)
	c.Assert(err, check.IsNil)
	diff, err := GetDiff(repo, string(firstHashCommit), string(secondHashCommit))
	c.Assert(err, check.IsNil)
	c.Assert(string(diff), check.Matches, `(?s).*-You should read this README.*\+Seriously, read this file!.*`)
}

func (s *S) TestGetDiffIntegrationWhenInvalidRepo(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "Just a regular readme."
	object1 := "You should read this README"
	object2 := "Seriously, read this file!"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	errCreateCommit := CreateCommit(bare, repo, file, object1)
	c.Assert(errCreateCommit, check.IsNil)
	firstHashCommit, err := GetLastHashCommit(bare, repo)
	c.Assert(err, check.IsNil)
	errCreateCommit = CreateCommit(bare, repo, file, object2)
	c.Assert(errCreateCommit, check.IsNil)
	secondHashCommit, err := GetLastHashCommit(bare, repo)
	c.Assert(err, check.IsNil)
	expectedErr := fmt.Sprintf("Error when trying to obtain diff with commits %s and %s of repository invalid-repo (Repository does not exist).", secondHashCommit, firstHashCommit)
	_, err = GetDiff("invalid-repo", string(firstHashCommit), string(secondHashCommit))
	c.Assert(err.Error(), check.Equals, expectedErr)
}

func (s *S) TestGetDiffIntegrationWhenInvalidCommit(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "Just a regular readme."
	object1 := "You should read this README"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	errCreateCommit := CreateCommit(bare, repo, file, object1)
	c.Assert(errCreateCommit, check.IsNil)
	firstHashCommit, err := GetLastHashCommit(bare, repo)
	c.Assert(err, check.IsNil)
	expectedErr := fmt.Sprintf("Error when trying to obtain diff with commits %s and 12beu23eu23923ey32eiyeg2ye of repository %s (exit status 128).", firstHashCommit, repo)
	_, err = GetDiff(repo, "12beu23eu23923ey32eiyeg2ye", string(firstHashCommit))
	c.Assert(err.Error(), check.Equals, expectedErr)
}

func (s *S) TestGetTagsIntegration(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo-tags"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	testPath := path.Join(bare, repo+".git")
	errCreateTag := CreateTag(testPath, "0.1")
	c.Assert(errCreateTag, check.IsNil)
	errCreateCommit := CreateCommit(bare, repo, "", "")
	c.Assert(errCreateCommit, check.IsNil)
	errCreateTag = CreateTag(testPath, "0.2")
	c.Assert(errCreateTag, check.IsNil)
	tags, err := GetTags(repo)
	c.Assert(err, check.IsNil)
	c.Assert(tags[0].Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(tags[0].Name, check.Equals, "0.1")
	c.Assert(tags[0].Committer.Name, check.Equals, "doge")
	c.Assert(tags[0].Committer.Email, check.Equals, "<much@email.com>")
	c.Assert(tags[0].Author.Name, check.Equals, "doge")
	c.Assert(tags[0].Author.Email, check.Equals, "<much@email.com>")
	c.Assert(tags[0].Tagger.Name, check.Equals, "")
	c.Assert(tags[0].Tagger.Email, check.Equals, "")
	c.Assert(tags[0].Subject, check.Equals, "much WOW")
	c.Assert(tags[0].CreatedAt, check.Equals, tags[0].Author.Date)
	c.Assert(tags[0].Links.ZipArchive, check.Equals, GetArchiveUrl(repo, "0.1", "zip"))
	c.Assert(tags[0].Links.TarArchive, check.Equals, GetArchiveUrl(repo, "0.1", "tar.gz"))
	c.Assert(tags[1].Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(tags[1].Name, check.Equals, "0.2")
	c.Assert(tags[1].Committer.Name, check.Equals, "doge")
	c.Assert(tags[1].Committer.Email, check.Equals, "<much@email.com>")
	c.Assert(tags[1].Author.Name, check.Equals, "doge")
	c.Assert(tags[1].Author.Email, check.Equals, "<much@email.com>")
	c.Assert(tags[1].Tagger.Name, check.Equals, "")
	c.Assert(tags[1].Tagger.Email, check.Equals, "")
	c.Assert(tags[1].Subject, check.Equals, "")
	c.Assert(tags[1].Links.ZipArchive, check.Equals, GetArchiveUrl(repo, "0.2", "zip"))
	c.Assert(tags[1].Links.TarArchive, check.Equals, GetArchiveUrl(repo, "0.2", "tar.gz"))
	c.Assert(tags[1].CreatedAt, check.Equals, tags[1].Author.Date)
}

func (s *S) TestGetTagsAnnotatedTag(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo-push"
	file := "README"
	content := "much WOW"
	user := GitUser{
		Name:  "user",
		Email: "user@globo.com",
	}
	cleanUp, errCreate := CreateEmptyTestBareRepository(bare, repo)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	clone, cloneCleanUp, errClone := TempClone(repo)
	if cloneCleanUp != nil {
		defer cloneCleanUp()
	}
	c.Assert(errClone, check.IsNil)
	errWrite := ioutil.WriteFile(path.Join(clone, file), []byte(content), 0644)
	c.Assert(errWrite, check.IsNil)
	errAddAll := AddAll(clone)
	c.Assert(errAddAll, check.IsNil)
	errCommit := Commit(clone, "commit message", user, user)
	c.Assert(errCommit, check.IsNil)
	errCreateTag := CreateTag(clone, "0.1")
	c.Assert(errCreateTag, check.IsNil)
	errCreateAnnotatedTag := CreateAnnotatedTag(clone, "0.2", "much tag", user)
	c.Assert(errCreateAnnotatedTag, check.IsNil)
	errCreateTag = CreateTag(clone, "0.3")
	c.Assert(errCreateTag, check.IsNil)
	errCreateAnnotatedTag = CreateAnnotatedTag(clone, "0.4", "", user)
	c.Assert(errCreateAnnotatedTag, check.IsNil)
	errPush := Push(clone, "master")
	c.Assert(errPush, check.IsNil)
	errPushTags := PushTags(clone)
	c.Assert(errPushTags, check.IsNil)
	tags, err := GetTags(repo)
	c.Assert(err, check.IsNil)
	c.Assert(tags, check.HasLen, 4)
	c.Assert(tags[2].Tagger.Name, check.Equals, user.Name)
	c.Assert(tags[2].Tagger.Email, check.Equals, "<"+user.Email+">")
	c.Assert(tags[2].CreatedAt, check.Equals, tags[2].Tagger.Date)
	c.Assert(tags[2].Subject, check.Equals, "much tag")
}

func (s *S) TestGetArchiveUrl(c *check.C) {
	url := GetArchiveUrl("repo", "ref", "zip")
	c.Assert(url, check.Equals, fmt.Sprintf("/repository/%s/archive?ref=%s&format=%s", "repo", "ref", "zip"))
}

func (s *S) TestTempCloneIntegration(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo-clone"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	clone, cloneCleanUp, errClone := TempClone(repo)
	if cloneCleanUp != nil {
		defer cloneCleanUp()
	}
	c.Assert(errClone, check.IsNil)
	dstat, errStat := os.Stat(clone)
	c.Assert(dstat.IsDir(), check.Equals, true)
	fstat, errStat := os.Stat(path.Join(clone, file))
	c.Assert(fstat.IsDir(), check.Equals, false)
	c.Assert(errStat, check.IsNil)
}

func (s *S) TestTempCloneWhenRepoInvalid(c *check.C) {
	clone, cloneCleanUp, err := TempClone("invalid-repo")
	if cloneCleanUp != nil {
		defer cloneCleanUp()
	}
	c.Assert(err.Error(), check.Equals, "Error when trying to clone repository invalid-repo (Repository does not exist).")
	c.Assert(cloneCleanUp, check.IsNil)
	c.Assert(clone, check.HasLen, 0)
}

func (s *S) TestTempCloneWhenGitError(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo-clone"
	file := "README"
	cleanUp, errCreate := CreateEmptyTestRepository(bare, repo)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	tmpdir, err := commandmocker.Error("git", "much error", 1)
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	clone, cloneCleanUp, errClone := TempClone(repo)
	if cloneCleanUp != nil {
		defer cloneCleanUp()
	}
	expectedErr := fmt.Sprintf("Error when trying to clone repository %s into %s (exit status 1 [much error]).", repo, clone)
	c.Assert(errClone.Error(), check.Equals, expectedErr)
	dstat, errStat := os.Stat(clone)
	c.Assert(dstat.IsDir(), check.Equals, true)
	fstat, errStat := os.Stat(path.Join(clone, file))
	c.Assert(fstat, check.IsNil)
	c.Assert(errStat, check.NotNil)
}

func (s *S) TestCheckoutIntegration(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo-checkout"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	errCreateBranches := CreateBranchesOnTestRepository(bare, repo, "doge_bites")
	c.Assert(errCreateBranches, check.IsNil)
	branches, err := GetBranches(repo)
	c.Assert(err, check.IsNil)
	c.Assert(branches, check.HasLen, 2)
	clone, cloneCleanUp, errClone := TempClone(repo)
	if cloneCleanUp != nil {
		defer cloneCleanUp()
	}
	c.Assert(errClone, check.IsNil)
	errCheckout := Checkout(clone, "doge_bites", false)
	c.Assert(errCheckout, check.IsNil)
	errCheckout = Checkout(clone, "master", false)
	c.Assert(errCheckout, check.IsNil)
}

func (s *S) TestCheckoutBareRepoIntegration(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo-checkout"
	cleanUp, errCreate := CreateEmptyTestBareRepository(bare, repo)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	branches, err := GetBranches(repo)
	c.Assert(err, check.IsNil)
	c.Assert(branches, check.HasLen, 0)
	clone, cloneCleanUp, errClone := TempClone(repo)
	if cloneCleanUp != nil {
		defer cloneCleanUp()
	}
	c.Assert(errClone, check.IsNil)
	errCheckout := Checkout(clone, "doge_bites", false)
	expectedErr := fmt.Sprintf("Error when trying to checkout clone %s into branch doge_bites (exit status 1 [error: pathspec 'doge_bites' did not match any file(s) known to git.\n]).", clone)
	c.Assert(errCheckout.Error(), check.Equals, expectedErr)
	errCheckout = Checkout(clone, "doge_bites", true)
	c.Assert(errCheckout, check.IsNil)
}

func (s *S) TestCheckoutWhenBranchInvalid(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo-checkout"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	errCreateBranches := CreateBranchesOnTestRepository(bare, repo, "doge_bites")
	c.Assert(errCreateBranches, check.IsNil)
	branches, err := GetBranches(repo)
	c.Assert(err, check.IsNil)
	c.Assert(branches, check.HasLen, 2)
	clone, cloneCleanUp, errClone := TempClone(repo)
	if cloneCleanUp != nil {
		defer cloneCleanUp()
	}
	c.Assert(errClone, check.IsNil)
	errCheckout := Checkout(clone, "doge_bites", false)
	c.Assert(errCheckout, check.IsNil)
	errCheckout = Checkout(clone, "master", false)
	c.Assert(errCheckout, check.IsNil)
	expectedErr := fmt.Sprintf("Error when trying to checkout clone %s into branch invalid_branch (exit status 1 [error: pathspec 'invalid_branch' did not match any file(s) known to git.\n]).", clone)
	errCheckout = Checkout(clone, "invalid_branch", false)
	c.Assert(errCheckout.Error(), check.Equals, expectedErr)
}

func (s *S) TestCheckoutWhenCloneInvalid(c *check.C) {
	err := Checkout("invalid_clone", "doge_bites", false)
	c.Assert(err.Error(), check.Equals, "Error when trying to checkout clone invalid_clone into branch doge_bites (Clone does not exist).")
}

func (s *S) TestCheckoutWhenGitError(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo-checkout"
	file := "README"
	content := "will\tbark"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	clone, cloneCleanUp, errClone := TempClone(repo)
	if cloneCleanUp != nil {
		defer cloneCleanUp()
	}
	c.Assert(errClone, check.IsNil)
	errCheckout := Checkout(clone, "master", false)
	c.Assert(errCheckout, check.IsNil)
	tmpdir, err := commandmocker.Error("git", "much error", 1)
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	expectedErr := fmt.Sprintf("Error when trying to checkout clone %s into branch master (exit status 1 [much error]).", clone)
	errCheckout = Checkout(clone, "master", false)
	c.Assert(errCheckout.Error(), check.Equals, expectedErr)
}

func (s *S) TestAddAllIntegration(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo-add-all"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateEmptyTestRepository(bare, repo)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	clone, cloneCleanUp, errClone := TempClone(repo)
	if cloneCleanUp != nil {
		defer cloneCleanUp()
	}
	c.Assert(errClone, check.IsNil)
	errWrite := ioutil.WriteFile(path.Join(clone, file), []byte(content+content), 0644)
	c.Assert(errWrite, check.IsNil)
	errWrite = ioutil.WriteFile(clone+"/WOWME", []byte(content+content), 0644)
	c.Assert(errWrite, check.IsNil)
	errAddAll := AddAll(clone)
	c.Assert(errAddAll, check.IsNil)
	gitPath, err := exec.LookPath("git")
	c.Assert(err, check.IsNil)
	cmd := exec.Command(gitPath, "diff", "--staged", "--stat")
	cmd.Dir = clone
	out, err := cmd.CombinedOutput()
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(string(out), file), check.Equals, true)
	c.Assert(strings.Contains(string(out), "WOWME"), check.Equals, true)
}

func (s *S) TestAddAllWhenCloneInvalid(c *check.C) {
	err := AddAll("invalid_clone")
	c.Assert(err.Error(), check.Equals, "Error when trying to add all to clone invalid_clone (Clone does not exist).")
}

func (s *S) TestAddAllWhenGitError(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo-add-all"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	clone, cloneCleanUp, errClone := TempClone(repo)
	if cloneCleanUp != nil {
		defer cloneCleanUp()
	}
	c.Assert(errClone, check.IsNil)
	errAddAll := AddAll(clone)
	c.Assert(errAddAll, check.IsNil)
	tmpdir, err := commandmocker.Error("git", "much error", 1)
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	expectedErr := fmt.Sprintf("Error when trying to add all to clone %s (exit status 1 [much error]).", clone)
	errAddAll = AddAll(clone)
	c.Assert(errAddAll.Error(), check.Equals, expectedErr)
}

func (s *S) TestCommitIntegration(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo-commit"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	clone, cloneCleanUp, errClone := TempClone(repo)
	if cloneCleanUp != nil {
		defer cloneCleanUp()
	}
	c.Assert(errClone, check.IsNil)
	errWrite := ioutil.WriteFile(path.Join(clone, file), []byte(content+content), 0644)
	c.Assert(errWrite, check.IsNil)
	gitPath, err := exec.LookPath("git")
	c.Assert(err, check.IsNil)
	cmd := exec.Command(gitPath, "diff", "--stat")
	cmd.Dir = clone
	out, err := cmd.CombinedOutput()
	c.Assert(err, check.IsNil)
	c.Assert(len(out) > 0, check.Equals, true)
	errAddAll := AddAll(clone)
	c.Assert(errAddAll, check.IsNil)
	author := GitUser{
		Name:  "author",
		Email: "author@globo.com",
	}
	committer := GitUser{
		Name:  "committer",
		Email: "committer@globo.com",
	}
	message := "commit message"
	errCommit := Commit(clone, message, author, committer)
	c.Assert(errCommit, check.IsNil)
	cmd = exec.Command(gitPath, "diff")
	cmd.Dir = clone
	out, err = cmd.CombinedOutput()
	c.Assert(err, check.IsNil)
	c.Assert(out, check.HasLen, 0)
}

func (s *S) TestCommitWhenCloneInvalid(c *check.C) {
	author := GitUser{
		Name:  "author",
		Email: "author@globo.com",
	}
	message := "commit message"
	err := Commit("invalid_clone", message, author, author)
	c.Assert(err.Error(), check.Equals, "Error when trying to commit to clone invalid_clone (Clone does not exist).")
}

func (s *S) TestCommitWhenGitError(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo-add-all"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	clone, cloneCleanUp, errClone := TempClone(repo)
	if cloneCleanUp != nil {
		defer cloneCleanUp()
	}
	c.Assert(errClone, check.IsNil)
	author := GitUser{
		Name:  "author",
		Email: "author@globo.com",
	}
	message := "commit message"
	tmpdir, err := commandmocker.Error("git", "much error", 1)
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	expectedErr := fmt.Sprintf("Error when trying to commit to clone %s (exit status 1 [much error]).", clone)
	errCommit := Commit(clone, message, author, author)
	c.Assert(errCommit.Error(), check.Equals, expectedErr)
}

func (s *S) TestPushIntegration(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo-push"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateEmptyTestBareRepository(bare, repo)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	clone, cloneCleanUp, errClone := TempClone(repo)
	if cloneCleanUp != nil {
		defer cloneCleanUp()
	}
	c.Assert(errClone, check.IsNil)
	errWrite := ioutil.WriteFile(path.Join(clone, file), []byte(content+content), 0644)
	c.Assert(errWrite, check.IsNil)
	errAddAll := AddAll(clone)
	c.Assert(errAddAll, check.IsNil)
	author := GitUser{
		Name:  "author",
		Email: "author@globo.com",
	}
	committer := GitUser{
		Name:  "committer",
		Email: "committer@globo.com",
	}
	message := "commit message"
	errCommit := Commit(clone, message, author, committer)
	c.Assert(errCommit, check.IsNil)
	errPush := Push(clone, "master")
	c.Assert(errPush, check.IsNil)
}

func (s *S) TestPushWhenCloneInvalid(c *check.C) {
	err := Push("invalid_clone", "master")
	c.Assert(err.Error(), check.Equals, "Error when trying to push clone invalid_clone into origin's master branch (Clone does not exist).")
}

func (s *S) TestPushWhenGitError(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo-add-all"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	clone, cloneCleanUp, errClone := TempClone(repo)
	if cloneCleanUp != nil {
		defer cloneCleanUp()
	}
	c.Assert(errClone, check.IsNil)
	tmpdir, err := commandmocker.Error("git", "much error", 1)
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	expectedErr := fmt.Sprintf("Error when trying to push clone %s into origin's master branch (exit status 1 [much error]).", clone)
	errPush := Push(clone, "master")
	c.Assert(errPush.Error(), check.Equals, expectedErr)
}

func (s *S) TestCommitZipIntegration(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo-push"
	cleanUp, errCreate := CreateEmptyTestBareRepository(bare, repo)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	boundary := "muchBOUNDARY"
	params := map[string]string{}
	var files = []multipartzip.File{
		{"doge.txt", "Much doge"},
		{"much.txt", "Much mucho"},
		{"much/WOW.txt", "Much WOW"},
	}
	buf, err := multipartzip.CreateZipBuffer(files)
	c.Assert(err, check.IsNil)
	reader, writer := io.Pipe()
	go multipartzip.StreamWriteMultipartForm(params, "muchfile", "muchfile.zip", boundary, writer, buf)
	mpr := multipart.NewReader(reader, boundary)
	form, err := mpr.ReadForm(0)
	c.Assert(err, check.IsNil)
	file, err := multipartzip.FileField(form, "muchfile")
	c.Assert(err, check.IsNil)
	commit := GitCommit{
		Message: "will bark",
		Author: GitUser{
			Name:  "author",
			Email: "author@globo.com",
		},
		Committer: GitUser{
			Name:  "committer",
			Email: "committer@globo.com",
		},
		Branch: "doge_barks",
	}
	ref, err := CommitZip(repo, file, commit)
	c.Assert(err, check.IsNil)
	c.Assert(ref.Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(ref.Name, check.Equals, "doge_barks")
	c.Assert(ref.Committer.Name, check.Equals, "committer")
	c.Assert(ref.Committer.Email, check.Equals, "<committer@globo.com>")
	c.Assert(ref.Author.Name, check.Equals, "author")
	c.Assert(ref.Author.Email, check.Equals, "<author@globo.com>")
	c.Assert(ref.Subject, check.Equals, "will bark")
	tree, err := GetTree(repo, "doge_barks", "")
	c.Assert(err, check.IsNil)
	c.Assert(tree, check.HasLen, 3)
	c.Assert(tree[0]["path"], check.Equals, "doge.txt")
	c.Assert(tree[0]["rawPath"], check.Equals, "doge.txt")
	c.Assert(tree[1]["path"], check.Equals, "much.txt")
	c.Assert(tree[1]["rawPath"], check.Equals, "much.txt")
	c.Assert(tree[2]["path"], check.Equals, "much/WOW.txt")
	c.Assert(tree[2]["rawPath"], check.Equals, "much/WOW.txt")
}

func (s *S) TestCommitZipIntegrationWhenFileEmpty(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo-push"
	cleanUp, errCreate := CreateEmptyTestBareRepository(bare, repo)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	boundary := "muchBOUNDARY"
	params := map[string]string{}
	reader, writer := io.Pipe()
	go multipartzip.StreamWriteMultipartForm(params, "muchfile", "muchfile.zip", boundary, writer, nil)
	mpr := multipart.NewReader(reader, boundary)
	form, err := mpr.ReadForm(0)
	c.Assert(err, check.IsNil)
	file, err := multipartzip.FileField(form, "muchfile")
	c.Assert(err, check.IsNil)
	commit := GitCommit{
		Message: "will bark",
		Author: GitUser{
			Name:  "author",
			Email: "author@globo.com",
		},
		Committer: GitUser{
			Name:  "committer",
			Email: "committer@globo.com",
		},
		Branch: "doge_barks",
	}
	expectedErr := fmt.Sprintf("Error when trying to commit zip to repository %s, could not extract: zip: not a valid zip file", repo)
	_, err = CommitZip(repo, file, commit)
	c.Assert(err.Error(), check.Equals, expectedErr)
}

func (s *S) TestGetLogs(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "will\tbark"
	object1 := "You should read this README"
	object2 := "Seriously, read this file!"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	errCreateCommit := CreateCommit(bare, repo, file, object1)
	c.Assert(errCreateCommit, check.IsNil)
	errCreateCommit = CreateCommit(bare, repo, file, object2)
	c.Assert(errCreateCommit, check.IsNil)
	history, err := GetLogs(repo, "HEAD", 1, "")
	c.Assert(err, check.IsNil)
	c.Assert(history.Commits, check.HasLen, 1)
	c.Assert(history.Commits[0].Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(history.Commits[0].Parent, check.HasLen, 1)
	c.Assert(history.Commits[0].Parent[0], check.Matches, "[a-f0-9]{40}")
	c.Assert(history.Commits[0].Committer.Name, check.Equals, "doge")
	c.Assert(history.Commits[0].Committer.Email, check.Equals, "much@email.com")
	c.Assert(history.Commits[0].Author.Name, check.Equals, "doge")
	c.Assert(history.Commits[0].Author.Email, check.Equals, "much@email.com")
	c.Assert(history.Commits[0].Subject, check.Equals, "Seriously, read this file!")
	c.Assert(history.Commits[0].CreatedAt, check.Equals, history.Commits[0].Author.Date)
	c.Assert(history.Next, check.Matches, "[a-f0-9]{40}")
	// Next
	history, err = GetLogs(repo, history.Next, 1, "")
	c.Assert(err, check.IsNil)
	c.Assert(history.Commits, check.HasLen, 1)
	c.Assert(history.Commits[0].Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(history.Commits[0].Parent, check.HasLen, 1)
	c.Assert(history.Commits[0].Parent[0], check.Matches, "[a-f0-9]{40}")
	c.Assert(history.Commits[0].Committer.Name, check.Equals, "doge")
	c.Assert(history.Commits[0].Committer.Email, check.Equals, "much@email.com")
	c.Assert(history.Commits[0].Author.Name, check.Equals, "doge")
	c.Assert(history.Commits[0].Author.Email, check.Equals, "much@email.com")
	c.Assert(history.Commits[0].Subject, check.Equals, "You should read this README")
	c.Assert(history.Commits[0].CreatedAt, check.Equals, history.Commits[0].Author.Date)
	c.Assert(history.Next, check.Matches, "[a-f0-9]{40}")
	// Next
	history, err = GetLogs(repo, history.Next, 1, "")
	c.Assert(err, check.IsNil)
	c.Assert(history.Commits, check.HasLen, 1)
	c.Assert(history.Commits[0].Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(history.Commits[0].Parent, check.HasLen, 0)
	c.Assert(history.Commits[0].Committer.Name, check.Equals, "doge")
	c.Assert(history.Commits[0].Committer.Email, check.Equals, "much@email.com")
	c.Assert(history.Commits[0].Author.Name, check.Equals, "doge")
	c.Assert(history.Commits[0].Author.Email, check.Equals, "much@email.com")
	c.Assert(history.Commits[0].Subject, check.Equals, "will\tbark")
	c.Assert(history.Commits[0].CreatedAt, check.Equals, history.Commits[0].Author.Date)
	c.Assert(history.Next, check.Equals, "")
}

func (s *S) TestGetLogsWithFile(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "will bark"
	object1 := "You should read this README"
	object2 := "Seriously, read this file!"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	errCreateCommit := CreateCommit(bare, repo, file, object1)
	c.Assert(errCreateCommit, check.IsNil)
	errCreateCommit = CreateCommit(bare, repo, file, object2)
	c.Assert(errCreateCommit, check.IsNil)
	history, err := GetLogs(repo, "master", 1, "README")
	c.Assert(err, check.IsNil)
	c.Assert(history.Commits, check.HasLen, 1)
	c.Assert(history.Commits[0].Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(history.Commits[0].Parent, check.HasLen, 1)
	c.Assert(history.Commits[0].Parent[0], check.Matches, "[a-f0-9]{40}")
	c.Assert(history.Commits[0].Committer.Name, check.Equals, "doge")
	c.Assert(history.Commits[0].Committer.Email, check.Equals, "much@email.com")
	c.Assert(history.Commits[0].Author.Name, check.Equals, "doge")
	c.Assert(history.Commits[0].Author.Email, check.Equals, "much@email.com")
	c.Assert(history.Commits[0].Subject, check.Equals, "Seriously, read this file!")
	c.Assert(history.Commits[0].CreatedAt, check.Equals, history.Commits[0].Author.Date)
	c.Assert(history.Next, check.Matches, "[a-f0-9]{40}")
}

func (s *S) TestGetLogsWithFileAndEmptyParameters(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "will bark"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	history, err := GetLogs(repo, "", 0, "")
	c.Assert(err, check.IsNil)
	c.Assert(history.Commits, check.HasLen, 1)
	c.Assert(history.Commits[0].Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(history.Commits[0].Parent, check.HasLen, 0)
	c.Assert(history.Commits[0].Committer.Name, check.Equals, "doge")
	c.Assert(history.Commits[0].Committer.Email, check.Equals, "much@email.com")
	c.Assert(history.Commits[0].Author.Name, check.Equals, "doge")
	c.Assert(history.Commits[0].Author.Email, check.Equals, "much@email.com")
	c.Assert(history.Commits[0].Subject, check.Equals, "will bark")
	c.Assert(history.Commits[0].CreatedAt, check.Equals, history.Commits[0].Author.Date)
	c.Assert(history.Next, check.Equals, "")
}

func (s *S) TestGetLogsWithAllSortsOfSubjects(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content1 := ""
	content2 := "will\tbark"
	content3 := "will bark"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content1)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	errCreateCommit := CreateCommit(bare, repo, file, content2)
	c.Assert(errCreateCommit, check.IsNil)
	errCreateCommit = CreateCommit(bare, repo, file, content3)
	c.Assert(errCreateCommit, check.IsNil)
	history, err := GetLogs(repo, "master", 3, "README")
	c.Assert(err, check.IsNil)
	c.Assert(history.Commits, check.HasLen, 3)
	c.Assert(history.Commits[0].Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(history.Commits[0].Parent, check.HasLen, 1)
	c.Assert(history.Commits[0].Parent[0], check.Matches, "[a-f0-9]{40}")
	c.Assert(history.Commits[0].Committer.Name, check.Equals, "doge")
	c.Assert(history.Commits[0].Committer.Email, check.Equals, "much@email.com")
	c.Assert(history.Commits[0].Author.Name, check.Equals, "doge")
	c.Assert(history.Commits[0].Author.Email, check.Equals, "much@email.com")
	c.Assert(history.Commits[0].Subject, check.Equals, "will bark")
	c.Assert(history.Commits[0].CreatedAt, check.Equals, history.Commits[0].Author.Date)
	c.Assert(history.Commits[1].Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(history.Commits[1].Parent, check.HasLen, 1)
	c.Assert(history.Commits[1].Parent[0], check.Matches, "[a-f0-9]{40}")
	c.Assert(history.Commits[1].Committer.Name, check.Equals, "doge")
	c.Assert(history.Commits[1].Committer.Email, check.Equals, "much@email.com")
	c.Assert(history.Commits[1].Author.Name, check.Equals, "doge")
	c.Assert(history.Commits[1].Author.Email, check.Equals, "much@email.com")
	c.Assert(history.Commits[1].Subject, check.Equals, "will\tbark")
	c.Assert(history.Commits[1].CreatedAt, check.Equals, history.Commits[1].Author.Date)
	c.Assert(history.Commits[2].Ref, check.Matches, "[a-f0-9]{40}")
	c.Assert(history.Commits[2].Parent, check.HasLen, 0)
	c.Assert(history.Commits[2].Committer.Name, check.Equals, "doge")
	c.Assert(history.Commits[2].Committer.Email, check.Equals, "much@email.com")
	c.Assert(history.Commits[2].Author.Name, check.Equals, "doge")
	c.Assert(history.Commits[2].Author.Email, check.Equals, "much@email.com")
	c.Assert(history.Commits[2].Subject, check.Equals, "")
	c.Assert(history.Commits[2].CreatedAt, check.Equals, history.Commits[2].Author.Date)
	c.Assert(history.Next, check.Equals, "")
}

func (s *S) TestGetLogsWhenOutputInvalid(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	tmpdir, err := commandmocker.Add("git", "-")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	_, err = GetLogs(repo, "master", 3, "README")
	c.Assert(err.Error(), check.Equals, "Error when trying to obtain the log of repository gandalf-test-repo (Invalid git log output [-]).")
}

func (s *S) TestGetLogsWhenOutputEmpty(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	tmpdir, err := commandmocker.Add("git", "\n")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	history, err := GetLogs(repo, "master", 1, "README")
	c.Assert(err, check.IsNil)
	c.Assert(history.Commits, check.HasLen, 0)
	c.Assert(history.Next, check.HasLen, 0)
}

func (s *S) TestGetLogsWhenGitError(c *check.C) {
	oldBare := bare
	bare = "/tmp"
	repo := "gandalf-test-repo"
	file := "README"
	content := "much WOW"
	cleanUp, errCreate := CreateTestRepository(bare, repo, file, content)
	defer func() {
		cleanUp()
		bare = oldBare
	}()
	c.Assert(errCreate, check.IsNil)
	tmpdir, err := commandmocker.Error("git", "much error", 1)
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(tmpdir)
	expectedErr := fmt.Sprintf("Error when trying to obtain the log of repository %s (exit status 1).", repo)
	_, err = GetLogs(repo, "master", 1, "README")
	c.Assert(err.Error(), check.Equals, expectedErr)
}

func (s *S) TestGetLogsWhenRepoInvalid(c *check.C) {
	expectedErr := fmt.Sprintf("Error when trying to obtain the log of repository invalid-repo (Repository does not exist).")
	_, err := GetLogs("invalid-repo", "master", 1, "README")
	c.Assert(err.Error(), check.Equals, expectedErr)
}
