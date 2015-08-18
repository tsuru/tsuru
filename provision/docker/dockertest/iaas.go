// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dockertest

import (
	"sync"

	"github.com/tsuru/tsuru/iaas"
)

type TestHealerIaaS struct {
	sync.Mutex
	Addr   string
	Err    error
	DelErr error
	Addrs  []string
	Ports  []int
	AddrId int
}

func NewHealerIaaSConstructor(addr string, err error) func(string) iaas.IaaS {
	return func(name string) iaas.IaaS {
		return &TestHealerIaaS{Addr: addr, Err: err}
	}
}

func NewHealerIaaSConstructorWithInst(addr string) (func(string) iaas.IaaS, *TestHealerIaaS) {
	inst := &TestHealerIaaS{Addr: addr}
	return func(name string) iaas.IaaS {
		return inst
	}, inst
}

func NewMultiHealerIaaSConstructor(addrs []string, ports []int, err error) func(string) iaas.IaaS {
	return func(name string) iaas.IaaS {
		return &TestHealerIaaS{Addrs: addrs, Ports: ports, Err: err}
	}
}

func (t *TestHealerIaaS) DeleteMachine(m *iaas.Machine) error {
	if t.DelErr != nil {
		return t.DelErr
	}
	return nil
}

func (t *TestHealerIaaS) CreateMachine(params map[string]string) (*iaas.Machine, error) {
	t.Lock()
	defer t.Unlock()
	if t.Err != nil {
		return nil, t.Err
	}
	var port int
	addr := t.Addr
	if len(t.Addrs) > 0 {
		addr = t.Addrs[t.AddrId]
		if len(t.Ports) == len(t.Addrs) {
			port = t.Ports[t.AddrId]
		}
		t.AddrId = (t.AddrId + 1) % len(t.Addrs)
	}
	m := iaas.Machine{
		Id:      "m-" + addr,
		Status:  "running",
		Address: addr,
		Port:    port,
	}
	return &m, nil
}

func (t *TestHealerIaaS) Describe() string {
	return "iaas describe"
}
