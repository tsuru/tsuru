// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockermachine

import (
	"encoding/json"
	"testing"

	check "gopkg.in/check.v1"

	"github.com/docker/machine/drivers/amazonec2"
	"github.com/docker/machine/drivers/fakedriver"
	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/engine"
	"github.com/docker/machine/libmachine/host"
	"github.com/docker/machine/libmachine/persist/persisttest"
	"github.com/docker/machine/libmachine/state"
	"github.com/tsuru/tsuru/iaas"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

type fakeLibMachineAPI struct {
	*persisttest.FakeStore
	driverName string
	ec2Driver  *amazonec2.Driver
	closed     bool
}

func (f *fakeLibMachineAPI) NewHost(driverName string, rawDriver []byte) (*host.Host, error) {
	f.driverName = driverName
	var driverOpts map[string]interface{}
	json.Unmarshal(rawDriver, &driverOpts)
	var driver drivers.Driver
	if driverName == "amazonec2" {
		driver = amazonec2.NewDriver("", "")
	} else {
		driver = &fakedriver.Driver{}
	}
	var name string
	if m, ok := driverOpts["MachineName"]; ok {
		name = m.(string)
	} else {
		name = driverOpts["MockName"].(string)
	}
	return &host.Host{
		Name:   name,
		Driver: driver,
		HostOptions: &host.Options{
			EngineOptions: &engine.Options{},
		},
	}, nil
}

func (f *fakeLibMachineAPI) Create(h *host.Host) error {
	if f.driverName == "amazonec2" {
		f.ec2Driver = h.Driver.(*amazonec2.Driver)
	}
	h.Driver = &fakedriver.Driver{
		MockName:  h.Name,
		MockState: state.Running,
		MockIP:    "192.168.10.3",
	}
	if f.FakeStore == nil {
		f.FakeStore = &persisttest.FakeStore{
			Hosts: make([]*host.Host, 0),
		}
	}
	f.Save(h)
	return nil
}

func (f *fakeLibMachineAPI) Close() error {
	f.closed = true
	return nil
}

func (f *fakeLibMachineAPI) GetMachinesDir() string {
	return ""
}

type fakeDockerMachine struct {
	closed         bool
	deletedMachine *iaas.Machine
	createdMachine *iaas.Machine
	config         DockerMachineConfig
}

var fakeDM = &fakeDockerMachine{}

func newFakeDockerMachine(c DockerMachineConfig) (dockerMachineAPI, error) {
	fakeDM.deletedMachine = nil
	fakeDM.createdMachine = nil
	fakeDM.config = c
	fakeDM.closed = false
	return fakeDM, nil
}

func (f *fakeDockerMachine) Close() error {
	f.closed = true
	return nil
}

func (f *fakeDockerMachine) CreateMachine(name, driver string, driverOpts map[string]interface{}) (*iaas.Machine, error) {
	f.createdMachine = &iaas.Machine{
		Id: name,
	}
	return f.createdMachine, nil
}

func (f *fakeDockerMachine) DeleteMachine(m *iaas.Machine) error {
	f.deletedMachine = m
	return nil
}
