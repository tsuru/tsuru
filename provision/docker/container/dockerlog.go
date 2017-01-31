// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

import (
	"strconv"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/scopedconfig"
)

var (
	ErrLogDriverMandatory  = errors.New("log-driver is mandatory")
	ErrLogDriverBSNoParams = errors.New("bs log-driver do not accept log-opts, please use node-container-update to configure it.")
)

const (
	dockerLogBsDriver         = "bs"
	dockerLogConfigCollection = "logs"
)

type DockerLogConfig struct {
	Driver  string
	LogOpts map[string]string
}

func loadLogConfig() *scopedconfig.ScopedConfig {
	conf := scopedconfig.FindScopedConfig(dockerLogConfigCollection)
	conf.ShallowMerge = true
	return conf
}

func LogOpts(pool string) (string, map[string]string, error) {
	conf := loadLogConfig()
	var entry DockerLogConfig
	err := conf.Load(pool, &entry)
	if err != nil {
		return "", nil, err
	}
	if entry.Driver == "" || entry.Driver == dockerLogBsDriver {
		return "syslog", map[string]string{
			"syslog-address": "udp://localhost:" + strconv.Itoa(BsSysLogPort()),
		}, nil
	}
	return entry.Driver, entry.LogOpts, nil
}

func LogIsBS(pool string) (bool, error) {
	conf := loadLogConfig()
	var logConf DockerLogConfig
	err := conf.Load(pool, &logConf)
	if err != nil {
		return false, err
	}
	return logConf.Driver == "" || logConf.Driver == dockerLogBsDriver, nil
}

func LogLoadAll() (map[string]DockerLogConfig, error) {
	conf := loadLogConfig()
	var logConf map[string]DockerLogConfig
	err := conf.LoadAll(&logConf)
	return logConf, err
}

func (logConf *DockerLogConfig) validate() error {
	if logConf.Driver == "" {
		return ErrLogDriverMandatory
	}
	if logConf.Driver == dockerLogBsDriver {
		if len(logConf.LogOpts) > 0 {
			return ErrLogDriverBSNoParams
		}
	}
	return nil
}

func (logConf *DockerLogConfig) Save(pool string) error {
	conf := loadLogConfig()
	err := logConf.validate()
	if err != nil {
		return err
	}
	return conf.Save(pool, *logConf)
}
