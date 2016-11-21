// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockermachine

import (
	"errors"

	"github.com/tsuru/tsuru/iaas"
)

type FakeDockerMachine struct {
	deletedMachine *iaas.Machine
	createdMachine *Machine
	config         *DockerMachineConfig
	hostOpts       *CreateMachineOpts
	closed         bool
}

var FakeDM = &FakeDockerMachine{}

func NewFakeDockerMachine(c DockerMachineConfig) (DockerMachineAPI, error) {
	FakeDM.deletedMachine = nil
	FakeDM.createdMachine = nil
	FakeDM.config = &c
	FakeDM.closed = false
	return FakeDM, nil
}

func (f *FakeDockerMachine) Close() error {
	f.closed = true
	return nil
}

func (f *FakeDockerMachine) CreateMachine(opts CreateMachineOpts) (*Machine, error) {
	f.createdMachine = &Machine{
		Base: &iaas.Machine{
			Id: opts.Name,
		},
	}
	var errCreate error
	if v, ok := opts.Params["error"]; ok {
		errCreate = errors.New(v.(string))
	}
	f.hostOpts = &opts
	return f.createdMachine, errCreate
}

func (f *FakeDockerMachine) DeleteMachine(m *iaas.Machine) error {
	f.deletedMachine = m
	return nil
}

func (f *FakeDockerMachine) DeleteAll() error {
	return nil
}

func (f *FakeDockerMachine) RegisterMachine(opts RegisterMachineOpts) (*Machine, error) {
	return nil, nil
}

func (f *FakeDockerMachine) List() ([]*Machine, error) {
	return nil, nil
}
