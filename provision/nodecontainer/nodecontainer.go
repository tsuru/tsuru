// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodecontainer

import (
	"fmt"
	"strings"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/globalsign/mgo"
	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/scopedconfig"
)

const (
	nodeContainerCollection = "nodeContainer"
)

var (
	ErrNodeContainerNotFound = errors.New("node container not found")
	ErrNodeContainerNoName   = ValidationErr{message: "node container config name cannot be empty"}
)

type NodeContainerConfig struct {
	Name        string
	PinnedImage string
	Disabled    *bool
	Config      docker.Config
	HostConfig  docker.HostConfig
}

type NodeContainerConfigGroup struct {
	Name        string
	ConfigPools map[string]NodeContainerConfig
}

type NodeContainerConfigGroupSlice []NodeContainerConfigGroup

type ValidationErr struct {
	message string
}

func (n ValidationErr) Error() string {
	return n.message
}

func (l NodeContainerConfigGroupSlice) Len() int           { return len(l) }
func (l NodeContainerConfigGroupSlice) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l NodeContainerConfigGroupSlice) Less(i, j int) bool { return l[i].Name < l[j].Name }

func (c *NodeContainerConfig) validate(pool string) error {
	if c.Name == "" {
		return ErrNodeContainerNoName
	}
	base, err := LoadNodeContainer("", c.Name)
	if err != nil {
		return err
	}
	if c.Config.Image == "" && (pool == "" || base.Config.Image == "") {
		return ValidationErr{message: "node container config image cannot be empty"}
	}
	return nil
}

func AddNewContainer(pool string, c *NodeContainerConfig) error {
	if err := c.validate(pool); err != nil {
		return err
	}
	conf := configFor(c.Name)
	return conf.Save(pool, c)
}

func UpdateContainer(pool string, c *NodeContainerConfig) error {
	if c.Name == "" {
		return ErrNodeContainerNoName
	}
	conf := configFor(c.Name)
	conf.SliceAdd = false
	conf.PtrNilIsEmpty = false
	hasEntry, err := conf.HasEntry(pool)
	if err != nil {
		return err
	}
	if !hasEntry {
		return ErrNodeContainerNotFound
	}
	err = removeDuplicateEnvs(c, conf, pool)
	if err != nil {
		return err
	}
	err = conf.SaveMerge(pool, c)
	if err != nil {
		return err
	}
	if c.Disabled != nil {
		return conf.SetField(pool, "Disabled", *c.Disabled)
	}
	return nil
}

func removeDuplicateEnvs(c *NodeContainerConfig, oldConf *scopedconfig.ScopedConfig, pool string) error {
	var oldPoolConf map[string]NodeContainerConfig
	err := oldConf.LoadPoolsMerge([]string{pool}, &oldPoolConf, false, false)
	if err != nil {
		return err
	}
	envMap := map[string]string{}
	allEnvs := append(oldPoolConf[pool].Config.Env, c.Config.Env...)
	for i := len(allEnvs) - 1; i >= 0; i-- {
		name := strings.Split(allEnvs[i], "=")[0]
		if _, ok := envMap[name]; !ok {
			envMap[name] = allEnvs[i]
		}
	}
	allEnvs = allEnvs[:0]
	for _, env := range envMap {
		allEnvs = append(allEnvs, env)
	}
	c.Config.Env = allEnvs
	return nil
}

func RemoveContainer(pool string, name string) error {
	conf := configFor(name)
	err := conf.Remove(pool)
	if err == mgo.ErrNotFound {
		return ErrNodeContainerNotFound
	}
	return err
}

func UpgradeContainer(pool string, name string) error {
	conf := configFor(name)
	hasEntry, err := conf.HasEntry(pool)
	if err != nil {
		return err
	}
	if !hasEntry {
		hasBaseEntry, err := conf.HasEntry("")
		if err != nil {
			return err
		}
		if !hasBaseEntry {
			return ErrNodeContainerNotFound
		}
		if pool != "" {
			err = AddNewContainer(pool, &NodeContainerConfig{Name: name})
			if err != nil {
				return err
			}
		}
	}
	return resetImage(pool, name)
}

func resetImage(pool string, name string) error {
	conf := configFor(name)
	var poolsToReset []string
	if pool == "" {
		poolMap, err := LoadNodeContainersForPools(name)
		if err != nil {
			return err
		}
		for poolName := range poolMap {
			poolsToReset = append(poolsToReset, poolName)
		}
	} else {
		poolsToReset = []string{pool}
	}
	for _, pool = range poolsToReset {
		var poolResult, base NodeContainerConfig
		err := conf.LoadWithBase(pool, &base, &poolResult)
		if err != nil {
			return err
		}
		var setPool string
		if poolResult.Image() != base.Image() {
			setPool = pool
		}
		err = conf.SetField(setPool, "PinnedImage", "")
		if err != nil {
			return err
		}
	}
	return nil
}

func LoadNodeContainer(pool string, name string) (*NodeContainerConfig, error) {
	conf := configFor(name)
	var result NodeContainerConfig
	err := conf.Load(pool, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func LoadNodeContainersForPools(name string) (map[string]NodeContainerConfig, error) {
	result, err := LoadNodeContainersForPoolsMerge(name, false)
	if err != nil {
		return result, err
	}
	if len(result) == 0 {
		return nil, ErrNodeContainerNotFound
	}
	return result, nil
}

func LoadNodeContainersForPoolsMerge(name string, merge bool) (map[string]NodeContainerConfig, error) {
	conf := configFor(name)
	var result map[string]NodeContainerConfig
	err := conf.LoadPoolsMerge(nil, &result, merge, false)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func AllNodeContainers() ([]NodeContainerConfigGroup, error) {
	confNames, err := scopedconfig.FindAllScopedConfigNames(nodeContainerCollection)
	if err != nil {
		return nil, err
	}
	result := make([]NodeContainerConfigGroup, len(confNames))
	for i, n := range confNames {
		confMap, err := LoadNodeContainersForPools(n)
		if err != nil {
			return nil, err
		}
		result[i] = NodeContainerConfigGroup{Name: n, ConfigPools: confMap}
	}
	return result, nil
}

func (c *NodeContainerConfig) EnvMap() map[string]string {
	envMap := map[string]string{}
	for _, e := range c.Config.Env {
		parts := strings.SplitN(e, "=", 2)
		envMap[parts[0]] = parts[1]
	}
	return envMap
}

func (c *NodeContainerConfig) Valid() bool {
	if c.Disabled != nil && *c.Disabled {
		return false
	}
	return c.Image() != ""
}

func (c *NodeContainerConfig) Image() string {
	if c.PinnedImage != "" {
		return c.PinnedImage
	}
	return c.Config.Image
}

func configFor(name string) *scopedconfig.ScopedConfig {
	conf := scopedconfig.FindScopedConfigFor(nodeContainerCollection, name)
	conf.Jsonfy = true
	conf.SliceAdd = true
	conf.AllowMapEmpty = true
	conf.PtrNilIsEmpty = true
	return conf
}

func shouldPinImage(image string) bool {
	parts := strings.SplitN(image, "/", 3)
	lastPart := parts[len(parts)-1]
	versionParts := strings.SplitN(lastPart, ":", 2)
	return len(versionParts) < 2 || versionParts[1] == "latest"
}

func (c *NodeContainerConfig) PinImageIfNeeded(image, digest, pool string) error {
	if !shouldPinImage(image) {
		return nil
	}
	base, err := LoadNodeContainer("", c.Name)
	if err != nil {
		return err
	}
	var pinToPool string
	if base.Image() != image {
		pinToPool = pool
	}
	var pinnedImage string
	if digest != "" {
		pinnedImage = fmt.Sprintf("%s@%s", image, digest)
	}
	if pinnedImage != image {
		c.PinnedImage = pinnedImage
		conf := configFor(c.Name)
		err = conf.SetField(pinToPool, "PinnedImage", pinnedImage)
	}
	return err
}

func AllNodeContainersNames() ([]string, error) {
	return scopedconfig.FindAllScopedConfigNames(nodeContainerCollection)
}
