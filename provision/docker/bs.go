// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"fmt"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const bsUniqueID = "bs"

type bsEnv struct {
	Name  string
	Value string
}

type bsPoolEnvs struct {
	Name string
	Envs []bsEnv
}

type bsConfig struct {
	ID    string `bson:"_id"`
	Image string
	Token string
	Envs  []bsEnv
	Pools []bsPoolEnvs
}

func (conf *bsConfig) updateEnvMaps(envMap map[string]string, poolEnvMap map[string]map[string]string) error {
	forbiddenList := map[string]bool{
		"DOCKER_ENDPOINT":       true,
		"TSURU_ENDPOINT":        true,
		"SYSLOG_LISTEN_ADDRESS": true,
		"TSURU_TOKEN":           true,
	}
	for _, env := range conf.Envs {
		if forbiddenList[env.Name] {
			return fmt.Errorf("cannot set %s variable", env.Name)
		}
		if env.Value == "" {
			delete(envMap, env.Name)
		} else {
			envMap[env.Name] = env.Value
		}
	}
	for _, p := range conf.Pools {
		if poolEnvMap[p.Name] == nil {
			poolEnvMap[p.Name] = make(map[string]string)
		}
		for _, env := range p.Envs {
			if forbiddenList[env.Name] {
				return fmt.Errorf("cannot set %s variable", env.Name)
			}
			if env.Value == "" {
				delete(poolEnvMap[p.Name], env.Name)
			} else {
				poolEnvMap[p.Name][env.Name] = env.Value
			}
		}
	}
	return nil
}

func (conf *bsConfig) getImage() string {
	if conf != nil && conf.Image != "" {
		return conf.Image
	}
	bsImage, _ := config.GetString("docker:bs:image")
	if bsImage == "" {
		bsImage = "tsuru/bs"
	}
	return bsImage
}

func (conf *bsConfig) envListForEndpoint(dockerEndpoint, poolName string) ([]string, error) {
	tsuruEndpoint, _ := config.GetString("host")
	if !strings.HasPrefix(tsuruEndpoint, "http://") && !strings.HasPrefix(tsuruEndpoint, "https://") {
		tsuruEndpoint = "http://" + tsuruEndpoint
	}
	tsuruEndpoint = strings.TrimRight(tsuruEndpoint, "/") + "/"
	endpoint := dockerEndpoint
	socket, _ := config.GetString("docker:bs:socket")
	if socket != "" {
		endpoint = "unix:///var/run/docker.sock"
	}
	token, err := conf.getToken()
	if err != nil {
		return nil, err
	}
	envList := []string{
		"DOCKER_ENDPOINT=" + endpoint,
		"TSURU_ENDPOINT=" + tsuruEndpoint,
		"TSURU_TOKEN=" + token,
		"SYSLOG_LISTEN_ADDRESS=udp://0.0.0.0:514",
	}
	envMap := make(map[string]string)
	poolEnvMap := make(map[string]map[string]string)
	conf.updateEnvMaps(envMap, poolEnvMap)
	for envName, envValue := range envMap {
		envList = append(envList, fmt.Sprintf("%s=%s", envName, envValue))
	}
	for envName, envValue := range poolEnvMap[poolName] {
		envList = append(envList, fmt.Sprintf("%s=%s", envName, envValue))
	}
	return envList, nil
}

func (conf *bsConfig) getToken() (string, error) {
	if conf.Token != "" {
		return conf.Token, nil
	}
	coll, err := bsCollection()
	if err != nil {
		return "", err
	}
	defer coll.Close()
	tokenData, err := app.AuthScheme.AppLogin(app.InternalAppName)
	if err != nil {
		return "", err
	}
	token := tokenData.GetValue()
	_, err = coll.Upsert(bson.M{
		"_id": bsUniqueID,
		"$or": []bson.M{{"token": ""}, {"token": bson.M{"$exists": false}}},
	}, bson.M{"$set": bson.M{"token": token}})
	if err == nil {
		conf.Token = token
		return token, nil
	}
	app.AuthScheme.Logout(token)
	if !mgo.IsDup(err) {
		return "", err
	}
	err = coll.FindId(bsUniqueID).One(conf)
	if err != nil {
		return "", err
	}
	return conf.Token, nil
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
	return bsConfig.getImage(), nil
}

func saveBsImage(digest string) error {
	coll, err := bsCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	_, err = coll.UpsertId(bsUniqueID, bson.M{"$set": bson.M{"image": digest}})
	return err
}

func saveBsEnvs(envMap map[string]string, poolEnvMap map[string]map[string]string) error {
	finalConf := bsConfigFromEnvMaps(envMap, poolEnvMap)
	coll, err := bsCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	_, err = coll.UpsertId(bsUniqueID, bson.M{"$set": bson.M{"envs": finalConf.Envs, "pools": finalConf.Pools}})
	return err
}

func loadBsConfig() (*bsConfig, error) {
	var config bsConfig
	coll, err := bsCollection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	err = coll.FindId(bsUniqueID).One(&config)
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
