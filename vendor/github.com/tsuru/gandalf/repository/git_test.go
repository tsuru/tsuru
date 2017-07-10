// Copyright 2015 gandalf authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

import (
	"fmt"
	"path"

	"github.com/tsuru/commandmocker"
	"github.com/tsuru/config"
	"github.com/tsuru/gandalf/fs"
	"github.com/tsuru/tsuru/fs/fstest"
	"gopkg.in/check.v1"
)

func (s *S) TestBareLocationValuShouldComeFromGandalfConf(c *check.C) {
	bare = ""
	config.Set("git:bare:location", "/home/gandalf")
	l := bareLocation()
	c.Assert(l, check.Equals, "/home/gandalf")
}

func (s *S) TestBareLocationShouldResetBareValue(c *check.C) {
	l := bareLocation()
	config.Set("git:bare:location", "fooo/baaar")
	c.Assert(bareLocation(), check.Equals, l)
}

func (s *S) TestNewBareShouldCreateADir(c *check.C) {
	dir, err := commandmocker.Add("git", "$*")
	c.Check(err, check.IsNil)
	defer commandmocker.Remove(dir)
	err = newBare("myBare")
	c.Assert(err, check.IsNil)
	c.Assert(commandmocker.Ran(dir), check.Equals, true)
}

func (s *S) TestNewBareShouldReturnMeaningfullErrorWhenBareCreationFails(c *check.C) {
	dir, err := commandmocker.Error("git", "cmd output", 1)
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(dir)
	err = newBare("foo")
	c.Check(err, check.NotNil)
	got := err.Error()
	expected := "Could not create git bare repository: exit status 1. cmd output"
	c.Assert(got, check.Equals, expected)
}

func (s *S) TestNewBareShouldPassTemplateOptionWhenItExistsOnConfig(c *check.C) {
	bareTemplate := "/var/templates"
	bareLocation, err := config.GetString("git:bare:location")
	config.Set("git:bare:template", bareTemplate)
	defer config.Unset("git:bare:template")
	barePath := path.Join(bareLocation, "foo.git")
	dir, err := commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(dir)
	err = newBare("foo")
	c.Assert(err, check.IsNil)
	c.Assert(commandmocker.Ran(dir), check.Equals, true)
	expected := fmt.Sprintf("init %s --bare --template=%s", barePath, bareTemplate)
	c.Assert(commandmocker.Output(dir), check.Equals, expected)
}

func (s *S) TestNewBareShouldNotPassTemplateOptionWhenItsNotSetInConfig(c *check.C) {
	config.Unset("git:bare:template")
	bareLocation, err := config.GetString("git:bare:location")
	c.Assert(err, check.IsNil)
	barePath := path.Join(bareLocation, "foo.git")
	dir, err := commandmocker.Add("git", "$*")
	c.Assert(err, check.IsNil)
	defer commandmocker.Remove(dir)
	err = newBare("foo")
	c.Assert(err, check.IsNil)
	c.Assert(commandmocker.Ran(dir), check.Equals, true)
	expected := fmt.Sprintf("init %s --bare", barePath)
	c.Assert(commandmocker.Output(dir), check.Equals, expected)
}

func (s *S) TestRemoveBareShouldRemoveBareDirFromFileSystem(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "foo"}
	fs.Fsystem = rfs
	defer func() { fs.Fsystem = nil }()
	err := removeBare("myBare")
	c.Assert(err, check.IsNil)
	action := "removeall " + path.Join(bareLocation(), "myBare.git")
	c.Assert(rfs.HasAction(action), check.Equals, true)
}

func (s *S) TestRemoveBareShouldReturnDescriptiveErrorWhenRemovalFails(c *check.C) {
	rfs := &fstest.RecordingFs{FileContent: "foo"}
	fs.Fsystem = &fstest.FileNotFoundFs{RecordingFs: *rfs}
	defer func() { fs.Fsystem = nil }()
	err := removeBare("fooo")
	c.Assert(err, check.ErrorMatches, "^Could not remove git bare repository: .*")
}
