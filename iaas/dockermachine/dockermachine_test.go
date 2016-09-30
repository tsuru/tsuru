// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockermachine

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/docker/machine/drivers/amazonec2"

	check "gopkg.in/check.v1"
)

func (s *S) TestNewDockerMachine(c *check.C) {
	dmAPI, err := NewDockerMachine(DockerMachineConfig{InsecureRegistry: "registry.com"})
	defer dmAPI.Close()
	dm := dmAPI.(*DockerMachine)
	c.Assert(err, check.IsNil)
	c.Assert(dm.client, check.NotNil)
	pathInfo, err := os.Stat(dm.path)
	c.Assert(err, check.IsNil)
	c.Assert(pathInfo.IsDir(), check.Equals, true)
}

func (s *S) TestNewDockerMachineCopyCaFiles(c *check.C) {
	caPath, err := ioutil.TempDir("", "")
	c.Assert(err, check.IsNil)
	defer os.RemoveAll(caPath)
	err = ioutil.WriteFile(filepath.Join(caPath, "ca.pem"), []byte("ca content"), 0700)
	c.Assert(err, check.IsNil)
	err = ioutil.WriteFile(filepath.Join(caPath, "ca-key.pem"), []byte("ca key content"), 0700)
	c.Assert(err, check.IsNil)
	dmAPI, err := NewDockerMachine(DockerMachineConfig{CaPath: caPath})
	defer dmAPI.Close()
	dm := dmAPI.(*DockerMachine)
	c.Assert(err, check.IsNil)
	c.Assert(dm.client, check.NotNil)
	ca, err := ioutil.ReadFile(filepath.Join(dm.path, "ca.pem"))
	c.Assert(err, check.IsNil)
	caKey, err := ioutil.ReadFile(filepath.Join(dm.path, "ca-key.pem"))
	c.Assert(err, check.IsNil)
	c.Assert(string(ca), check.Equals, "ca content")
	c.Assert(string(caKey), check.Equals, "ca key content")
}

func (s *S) TestClose(c *check.C) {
	fakeAPI := &fakeLibMachineAPI{}
	dmAPI, err := NewDockerMachine(DockerMachineConfig{})
	defer dmAPI.Close()
	dm := dmAPI.(*DockerMachine)
	c.Assert(err, check.IsNil)
	dm.client = fakeAPI
	err = dm.Close()
	c.Assert(err, check.IsNil)
	c.Assert(fakeAPI.closed, check.Equals, true)
	pathInfo, err := os.Stat(dm.path)
	c.Assert(err, check.NotNil)
	c.Assert(pathInfo, check.IsNil)
}

func (s *S) TestCreateMachine(c *check.C) {
	fakeAPI := &fakeLibMachineAPI{}
	dmAPI, err := NewDockerMachine(DockerMachineConfig{
		InsecureRegistry:       "registry.com",
		DockerEngineInstallURL: "https://getdocker2.com",
	})
	defer dmAPI.Close()
	dm := dmAPI.(*DockerMachine)
	dm.client = fakeAPI
	driverOpts := map[string]interface{}{
		"amazonec2-access-key": "access-key",
		"amazonec2-secret-key": "secret-key",
		"amazonec2-subnet-id":  "subnet-id",
	}
	m, err := dm.CreateMachine("my-machine", "amazonec2", driverOpts)
	c.Assert(err, check.IsNil)
	c.Assert(len(fakeAPI.Hosts), check.Equals, 1)
	c.Assert(m.Id, check.Equals, "my-machine")
	c.Assert(m.Port, check.Equals, 2376)
	c.Assert(m.Protocol, check.Equals, "https")
	c.Assert(m.Address, check.Equals, "192.168.10.3")
	c.Assert(fakeAPI.driverName, check.Equals, "amazonec2")
	c.Assert(fakeAPI.ec2Driver.AccessKey, check.Equals, "access-key")
	c.Assert(fakeAPI.ec2Driver.SecretKey, check.Equals, "secret-key")
	c.Assert(fakeAPI.ec2Driver.SubnetId, check.Equals, "subnet-id")
	engineOpts := fakeAPI.Hosts[0].HostOptions.EngineOptions
	c.Assert(engineOpts.InsecureRegistry, check.DeepEquals, []string{"registry.com"})
	c.Assert(engineOpts.InstallURL, check.Equals, "https://getdocker2.com")
}

func (s *S) TestDeleteMachine(c *check.C) {
	fakeAPI := &fakeLibMachineAPI{}
	dmAPI, err := NewDockerMachine(DockerMachineConfig{})
	defer dmAPI.Close()
	dm := dmAPI.(*DockerMachine)
	dm.client = fakeAPI
	m, err := dm.CreateMachine("my-machine", "fakedriver", map[string]interface{}{})
	c.Assert(err, check.IsNil)
	c.Assert(len(fakeAPI.Hosts), check.Equals, 1)
	err = dm.DeleteMachine(m)
	c.Assert(err, check.IsNil)
	c.Assert(len(fakeAPI.Hosts), check.Equals, 0)
}

func (s *S) TestConfigureDriver(c *check.C) {
	opts := map[string]interface{}{
		"amazonec2-tags":           "my-tag1",
		"amazonec2-access-key":     "abc",
		"amazonec2-subnet-id":      "net",
		"amazonec2-security-group": []string{"sg-123", "sg-456"},
	}
	driver := amazonec2.NewDriver("", "")
	err := configureDriver(driver, opts)
	c.Assert(err, check.NotNil)
	opts["amazonec2-secret-key"] = "cde"
	err = configureDriver(driver, opts)
	c.Assert(err, check.IsNil)
	c.Assert(driver.SecurityGroupNames, check.DeepEquals, []string{"sg-123", "sg-456"})
	c.Assert(driver.SecretKey, check.Equals, "cde")
	c.Assert(driver.SubnetId, check.Equals, "net")
	c.Assert(driver.AccessKey, check.Equals, "abc")
	c.Assert(driver.RetryCount, check.Equals, 5)
	c.Assert(driver.Tags, check.Equals, "my-tag1")
}

func (s *S) TestCopy(c *check.C) {
	path, err := ioutil.TempDir("", "")
	c.Assert(err, check.IsNil)
	err = ioutil.WriteFile(filepath.Join(path, "src"), []byte("file contents"), 0700)
	c.Assert(err, check.IsNil)
	err = copy(filepath.Join(path, "src"), filepath.Join(path, "dst"))
	c.Assert(err, check.IsNil)
	contents, err := ioutil.ReadFile(filepath.Join(path, "dst"))
	c.Assert(err, check.IsNil)
	c.Assert(string(contents), check.Equals, "file contents")
}
