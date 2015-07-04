// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
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

type poolEnvs struct {
	Name string
	Envs []bsEnv
}

type bsConfig struct {
	Image string
	Envs  []bsEnv
	Pools []poolEnvs
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
