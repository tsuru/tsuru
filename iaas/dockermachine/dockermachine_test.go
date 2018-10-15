// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockermachine

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/docker/machine/drivers/amazonec2"
	"github.com/tsuru/tsuru/iaas"
	check "gopkg.in/check.v1"
)

func (s *S) TestNewDockerMachine(c *check.C) {
	dmAPI, err := NewDockerMachine(DockerMachineConfig{})
	c.Assert(err, check.IsNil)
	defer dmAPI.Close()
	dm := dmAPI.(*DockerMachine)
	c.Assert(dm.client, check.NotNil)
	pathInfo, err := os.Stat(dm.StorePath)
	c.Assert(err, check.IsNil)
	c.Assert(pathInfo.IsDir(), check.Equals, true)
}

func (s *S) TestNewDockerMachineCreatesStoreIfDefinedAndDoesNotExists(c *check.C) {
	storePath, err := ioutil.TempDir("", "")
	c.Assert(err, check.IsNil)
	err = os.RemoveAll(storePath)
	c.Assert(err, check.IsNil)
	dmAPI, err := NewDockerMachine(DockerMachineConfig{StorePath: storePath})
	c.Assert(err, check.IsNil)
	defer dmAPI.Close()
	dm := dmAPI.(*DockerMachine)
	pathInfo, err := os.Stat(dm.StorePath)
	c.Assert(err, check.IsNil)
	c.Assert(pathInfo.IsDir(), check.Equals, true)
	pathInfo, err = os.Stat(dm.CertsPath)
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
	c.Assert(err, check.IsNil)
	defer dmAPI.Close()
	dm := dmAPI.(*DockerMachine)
	c.Assert(dm.client, check.NotNil)
	ca, err := ioutil.ReadFile(filepath.Join(dm.CertsPath, "ca.pem"))
	c.Assert(err, check.IsNil)
	caKey, err := ioutil.ReadFile(filepath.Join(dm.CertsPath, "ca-key.pem"))
	c.Assert(err, check.IsNil)
	c.Assert(string(ca), check.Equals, "ca content")
	c.Assert(string(caKey), check.Equals, "ca key content")
}

func (s *S) TestClose(c *check.C) {
	fakeAPI := &fakeLibMachineAPI{}
	dmAPI, err := NewDockerMachine(DockerMachineConfig{})
	c.Assert(err, check.IsNil)
	defer dmAPI.Close()
	dm := dmAPI.(*DockerMachine)
	dm.client = fakeAPI
	err = dm.Close()
	c.Assert(err, check.IsNil)
	c.Assert(fakeAPI.closed, check.Equals, true)
	pathInfo, err := os.Stat(dm.StorePath)
	c.Assert(err, check.NotNil)
	c.Assert(pathInfo, check.IsNil)
}

func (s *S) TestCreateMachine(c *check.C) {
	fakeAPI := &fakeLibMachineAPI{}
	dmAPI, err := NewDockerMachine(DockerMachineConfig{})
	c.Assert(err, check.IsNil)
	defer dmAPI.Close()
	dm := dmAPI.(*DockerMachine)
	dm.client = fakeAPI
	driverOpts := map[string]interface{}{
		"amazonec2-access-key":     "access-key",
		"amazonec2-secret-key":     "secret-key",
		"amazonec2-subnet-id":      "subnet-id",
		"amazonec2-security-group": "sg1,sg2",
		"amazonec2-root-size":      "10",
	}
	opts := CreateMachineOpts{
		Name:                   "my-machine",
		DriverName:             "amazonec2",
		Params:                 driverOpts,
		InsecureRegistry:       "registry.com",
		DockerEngineInstallURL: "https://getdocker2.com",
		RegistryMirror:         "http://registry-mirror.com",
		ArbitraryFlags:         []string{"flag1", "flag2"},
	}
	m, err := dm.CreateMachine(opts)
	c.Assert(err, check.IsNil)
	base := m.Base
	c.Assert(base.Id, check.Equals, "my-machine")
	c.Assert(base.Port, check.Equals, 2376)
	c.Assert(base.Protocol, check.Equals, "https")
	c.Assert(base.Address, check.Equals, "192.168.10.3")
	c.Assert(string(base.CaCert), check.Equals, "ca")
	c.Assert(string(base.ClientCert), check.Equals, "cert")
	c.Assert(string(base.ClientKey), check.Equals, "key")
	c.Assert(len(fakeAPI.Hosts), check.Equals, 1)
	c.Assert(fakeAPI.driverName, check.Equals, "amazonec2")
	c.Assert(fakeAPI.ec2Driver.AccessKey, check.Equals, "access-key")
	c.Assert(fakeAPI.ec2Driver.SecretKey, check.Equals, "secret-key")
	c.Assert(fakeAPI.ec2Driver.SubnetId, check.Equals, "subnet-id")
	c.Assert(fakeAPI.ec2Driver.SecurityGroupNames, check.DeepEquals, []string{"sg1", "sg2"})
	c.Assert(fakeAPI.ec2Driver.RootSize, check.Equals, int64(10))
	engineOpts := fakeAPI.Hosts[0].HostOptions.EngineOptions
	c.Assert(engineOpts.InsecureRegistry, check.DeepEquals, []string{"registry.com"})
	c.Assert(engineOpts.InstallURL, check.Equals, "https://getdocker2.com")
	c.Assert(engineOpts.RegistryMirror, check.DeepEquals, []string{"http://registry-mirror.com"})
	c.Assert(m.Host, check.DeepEquals, fakeAPI.Hosts[0])
	c.Assert(engineOpts.ArbitraryFlags, check.DeepEquals, []string{"flag1", "flag2"})
}

func (s *S) TestDeleteMachine(c *check.C) {
	fakeAPI := &fakeLibMachineAPI{}
	dmAPI, err := NewDockerMachine(DockerMachineConfig{})
	c.Assert(err, check.IsNil)
	defer dmAPI.Close()
	dm := dmAPI.(*DockerMachine)
	dm.client = fakeAPI
	m, err := dm.CreateMachine(CreateMachineOpts{
		Name:       "my-machine",
		DriverName: "fakedriver",
		Params:     map[string]interface{}{},
	})
	c.Assert(err, check.IsNil)
	c.Assert(len(fakeAPI.Hosts), check.Equals, 1)
	err = dm.DeleteMachine(m.Base)
	c.Assert(err, check.IsNil)
	c.Assert(len(fakeAPI.Hosts), check.Equals, 0)
}

func (s *S) TestConfigureDriver(c *check.C) {
	opts := map[string]interface{}{
		"amazonec2-tags":                  "my-tag1",
		"amazonec2-access-key":            "abc",
		"amazonec2-subnet-id":             "net",
		"amazonec2-security-group":        []string{"sg-123", "sg-456"},
		"amazonec2-root-size":             "100",
		"amazonec2-request-spot-instance": "true",
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
	c.Assert(driver.RootSize, check.Equals, int64(100))
	c.Assert(driver.RequestSpotInstance, check.Equals, true)
}

func (s *S) TestDeleteAll(c *check.C) {
	fakeAPI := &fakeLibMachineAPI{}
	dmAPI, err := NewDockerMachine(DockerMachineConfig{})
	c.Assert(err, check.IsNil)
	defer dmAPI.Close()
	dm := dmAPI.(*DockerMachine)
	dm.client = fakeAPI
	_, err = dm.CreateMachine(CreateMachineOpts{
		Name:       "my-machine",
		DriverName: "fakedriver",
		Params:     map[string]interface{}{},
	})
	c.Assert(err, check.IsNil)
	_, err = dm.CreateMachine(CreateMachineOpts{
		Name:       "my-machine-2",
		DriverName: "fakedriver",
		Params:     map[string]interface{}{},
	})
	c.Assert(err, check.IsNil)
	err = dm.DeleteAll()
	c.Assert(err, check.IsNil)
}

func (s *S) TestRegisterMachine(c *check.C) {
	fakeAPI := &fakeLibMachineAPI{}
	dmAPI, err := NewDockerMachine(DockerMachineConfig{})
	c.Assert(err, check.IsNil)
	defer dmAPI.Close()
	dm := dmAPI.(*DockerMachine)
	dm.client = fakeAPI
	base := &iaas.Machine{
		CustomData: map[string]interface{}{
			"MachineName": "my-machine",
		},
		CaCert:     []byte("ca cert content"),
		ClientCert: []byte("client cert content"),
		ClientKey:  []byte("client key content"),
	}
	m, err := dm.RegisterMachine(RegisterMachineOpts{
		Base:          base,
		DriverName:    "amazonec2",
		SSHPrivateKey: []byte("private-key"),
	})
	c.Assert(err, check.IsNil)
	c.Assert(m.Base, check.DeepEquals, base)
	caCert, err := ioutil.ReadFile(m.Host.AuthOptions().CaCertPath)
	c.Assert(err, check.IsNil)
	c.Assert(caCert, check.DeepEquals, base.CaCert)
	clientCert, err := ioutil.ReadFile(m.Host.AuthOptions().ClientCertPath)
	c.Assert(err, check.IsNil)
	c.Assert(clientCert, check.DeepEquals, base.ClientCert)
	clientKey, err := ioutil.ReadFile(m.Host.AuthOptions().ClientKeyPath)
	c.Assert(err, check.IsNil)
	c.Assert(clientKey, check.DeepEquals, base.ClientKey)
	sshKey, err := ioutil.ReadFile(m.Host.Driver.GetSSHKeyPath())
	c.Assert(err, check.IsNil)
	c.Assert(sshKey, check.DeepEquals, []byte("private-key"))
}

func (s *S) TestList(c *check.C) {
	fakeAPI := &fakeLibMachineAPI{}
	dmAPI, err := NewDockerMachine(DockerMachineConfig{})
	c.Assert(err, check.IsNil)
	defer dmAPI.Close()
	dm := dmAPI.(*DockerMachine)
	dm.client = fakeAPI
	m, err := dm.CreateMachine(CreateMachineOpts{
		Name:                   "my-machine-1",
		DriverName:             "fakedriver",
		InsecureRegistry:       "registry.com",
		DockerEngineInstallURL: "https://getdocker2.com",
		RegistryMirror:         "http://registry-mirror.com",
	})
	c.Assert(err, check.IsNil)
	m2, err := dm.CreateMachine(CreateMachineOpts{
		Name:                   "my-machine-2",
		DriverName:             "fakedriver",
		InsecureRegistry:       "registry.com",
		DockerEngineInstallURL: "https://getdocker2.com",
		RegistryMirror:         "http://registry-mirror.com",
	})
	c.Assert(err, check.IsNil)
	machines, err := dm.List()
	c.Assert(err, check.IsNil)
	c.Assert(machines, check.DeepEquals, []*Machine{m, m2})
}
