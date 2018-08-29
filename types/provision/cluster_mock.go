// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

var _ ClusterStorage = &MockClusterStorage{}
var _ ClusterService = &MockClusterService{}

// MockClusterStorage implements ClusterStorage interface
type MockClusterStorage struct {
	OnUpsert            func(Cluster) error
	OnFindAll           func() ([]Cluster, error)
	OnFindByName        func(string) (*Cluster, error)
	OnFindByProvisioner func(string) ([]Cluster, error)
	OnFindByPool        func(string, string) (*Cluster, error)
	OnDelete            func(Cluster) error
}

func (m *MockClusterStorage) Upsert(c Cluster) error {
	if m.OnUpsert == nil {
		return nil
	}
	return m.OnUpsert(c)
}

func (m *MockClusterStorage) FindAll() ([]Cluster, error) {
	if m.OnFindAll == nil {
		return nil, nil
	}
	return m.OnFindAll()
}

func (m *MockClusterStorage) FindByName(name string) (*Cluster, error) {
	if m.OnFindByName == nil {
		return nil, nil
	}
	return m.OnFindByName(name)
}

func (m *MockClusterStorage) FindByProvisioner(provisionerName string) ([]Cluster, error) {
	if m.OnFindByProvisioner == nil {
		return nil, nil
	}
	return m.OnFindByProvisioner(provisionerName)
}

func (m *MockClusterStorage) FindByPool(provisioner, pool string) (*Cluster, error) {
	if m.OnFindByPool == nil {
		return nil, nil
	}
	return m.OnFindByPool(provisioner, pool)
}

func (m *MockClusterStorage) Delete(c Cluster) error {
	if m.OnDelete == nil {
		return nil
	}
	return m.OnDelete(c)
}

type MockClusterService struct {
	OnCreate            func(Cluster) error
	OnUpdate            func(Cluster) error
	OnList              func() ([]Cluster, error)
	OnFindByName        func(string) (*Cluster, error)
	OnFindByProvisioner func(string) ([]Cluster, error)
	OnFindByPool        func(string, string) (*Cluster, error)
	OnFindByPools       func(string, []string) (map[string]Cluster, error)
	OnDelete            func(Cluster) error
}

func (m *MockClusterService) Create(c Cluster) error {
	if m.OnCreate == nil {
		return nil
	}
	return m.OnCreate(c)
}

func (m *MockClusterService) Update(c Cluster) error {
	if m.OnUpdate == nil {
		return nil
	}
	return m.OnUpdate(c)
}

func (m *MockClusterService) List() ([]Cluster, error) {
	if m.OnList == nil {
		return nil, nil
	}
	return m.OnList()
}

func (m *MockClusterService) FindByName(name string) (*Cluster, error) {
	if m.OnFindByName == nil {
		return nil, nil
	}
	return m.OnFindByName(name)
}

func (m *MockClusterService) FindByProvisioner(prov string) ([]Cluster, error) {
	if m.OnFindByProvisioner == nil {
		return nil, nil
	}
	return m.OnFindByProvisioner(prov)
}

func (m *MockClusterService) FindByPool(prov, pool string) (*Cluster, error) {
	if m.OnFindByPool == nil {
		return nil, nil
	}
	return m.OnFindByPool(prov, pool)
}

func (m *MockClusterService) FindByPools(provisioner string, pool []string) (map[string]Cluster, error) {
	if m.OnFindByPools == nil {
		return nil, nil
	}
	return m.OnFindByPools(provisioner, pool)
}

func (m *MockClusterService) Delete(c Cluster) error {
	if m.OnDelete == nil {
		return nil
	}
	return m.OnDelete(c)
}
