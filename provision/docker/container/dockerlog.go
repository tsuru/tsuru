// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"errors"
	"strconv"

	"github.com/tsuru/tsuru/provision"
)

var (
	ErrLogDriverMandatory  = errors.New("log-driver is mandatory")
	ErrLogDriverBSNoParams = errors.New("bs log-driver do not accept log-opts, please use bs-env-set to configure it.")
)

const (
	DockerLogDriverConfig = "log-driver"
	dockerLogBsDriver     = "bs"
	dockerLogConfigEntry  = "logs"
)

type DockerLog struct {
	conf *provision.ScopedConfig
}

func (d *DockerLog) loadConfig() error {
	if d.conf != nil {
		return nil
	}
	var err error
	d.conf, err = provision.FindScopedConfig(dockerLogConfigEntry)
	if err != nil {
		return err
	}
	return nil
}

func (d *DockerLog) LogOpts(pool string) (string, map[string]string, error) {
	err := d.loadConfig()
	if err != nil {
		return "", nil, err
	}
	entryMap := d.conf.PoolEntries(pool)
	driver := entryMap[DockerLogDriverConfig]
	driverVal, _ := driver.Value.(string)
	if driverVal == "" || driverVal == dockerLogBsDriver {
		return "syslog", map[string]string{
			"syslog-address": "udp://localhost:" + strconv.Itoa(BsSysLogPort()),
		}, nil
	}
	logOpts := map[string]string{}
	delete(entryMap, DockerLogDriverConfig)
	for name, value := range entryMap {
		logOpts[name], _ = value.Value.(string)
	}
	return driverVal, logOpts, nil
}

func (d *DockerLog) validateEnvLogDriver(envs []provision.Entry) error {
	var driver string
	for _, env := range envs {
		if env.Name == DockerLogDriverConfig {
			driver, _ = env.Value.(string)
			break
		}
	}
	if driver == "" {
		return ErrLogDriverMandatory
	}
	if driver == dockerLogBsDriver {
		if len(envs) > 1 {
			return ErrLogDriverBSNoParams
		}
	}
	return nil
}

func (d *DockerLog) IsBS(pool string) (bool, error) {
	err := d.loadConfig()
	if err != nil {
		return false, err
	}
	driver := d.conf.PoolEntry(pool, DockerLogDriverConfig)
	return driver == dockerLogBsDriver || driver == "", nil
}

func (d *DockerLog) Update(toMerge *provision.ScopedConfig) error {
	err := d.loadConfig()
	if err != nil {
		return err
	}
	if len(toMerge.Envs) > 0 {
		d.conf.ResetBaseEnvs()
		err = d.validateEnvLogDriver(toMerge.Envs)
		if err != nil {
			return err
		}
	}
	for _, p := range toMerge.Pools {
		d.conf.ResetPoolEnvs(p.Name)
		err = d.validateEnvLogDriver(p.Envs)
		if err != nil {
			return err
		}
	}
	return d.conf.UpdateWith(toMerge)
}
