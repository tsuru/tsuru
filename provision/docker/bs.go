// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type bsEnv struct {
	Name  string
	Value string
}

type bsPoolEnvs struct {
	Name string
	Envs []bsEnv
}

type bsConfig struct {
	Image string
	Envs  []bsEnv
	Pools []bsPoolEnvs
}

func (conf *bsConfig) updateEnvMaps(envMap map[string]string, poolEnvMap map[string]map[string]string) error {
	forbiddenList := map[string]bool{
		"DOCKER_ENDPOINT":       true,
		"TSURU_ENDPOINT":        true,
		"TSURU_TOKEN":           true,
		"SYSLOG_LISTEN_ADDRESS": true,
	}
	for _, env := range conf.Envs {
		if forbiddenList[env.Name] {
			return fmt.Errorf("cannot set %s variable", env.Name)
		}
		envMap[env.Name] = env.Value
	}
	for _, p := range conf.Pools {
		if poolEnvMap[p.Name] == nil {
			poolEnvMap[p.Name] = make(map[string]string)
		}
		for _, env := range p.Envs {
			if forbiddenList[env.Name] {
				return fmt.Errorf("cannot set %s variable", env.Name)
			}
			poolEnvMap[p.Name][env.Name] = env.Value
		}
	}
	return nil
}

func bsConfigFromEnvMaps(envMap map[string]string, poolEnvMap map[string]map[string]string) *bsConfig {
	var finalConf bsConfig
	for name, value := range envMap {
		finalConf.Envs = append(finalConf.Envs, bsEnv{Name: name, Value: value})
	}
	for poolName, envMap := range poolEnvMap {
		poolEnv := bsPoolEnvs{Name: poolName}
		for name, value := range envMap {
			poolEnv.Envs = append(poolEnv.Envs, bsEnv{Name: name, Value: value})
		}
		finalConf.Pools = append(finalConf.Pools, poolEnv)
	}
	return &finalConf
}

func getBsSysLogPort() int {
	bsPort, _ := config.GetInt("docker:bs:syslog-port")
	if bsPort == 0 {
		bsPort = 1514
	}
	return bsPort
}

func getBsImage() (string, error) {
	bsConfig, err := loadBsConfig()
	if err != nil && err != mgo.ErrNotFound {
		return "", err
	}
	if bsConfig != nil {
		return bsConfig.Image, nil
	}
	bsImage, _ := config.GetString("docker:bs:image")
	if bsImage == "" {
		bsImage = "tsuru/bs"
	}
	return bsImage, nil
}

func saveBsImage(digest string) error {
	coll, err := bsCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	_, err = coll.Upsert(nil, bson.M{"$set": bson.M{"image": digest}})
	return err
}

func saveBsEnvs(envMap map[string]string, poolEnvMap map[string]map[string]string) error {
	finalConf := bsConfigFromEnvMaps(envMap, poolEnvMap)
	coll, err := bsCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	_, err = coll.Upsert(nil, bson.M{"$set": bson.M{"envs": finalConf.Envs, "pools": finalConf.Pools}})
	return err
}

func loadBsConfig() (*bsConfig, error) {
	var config bsConfig
	coll, err := bsCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	err = coll.Find(nil).One(&config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func bsCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	return conn.Collection("bsconfig"), nil
}
