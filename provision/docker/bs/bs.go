// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/fsouza/go-dockerclient"
	"github.com/tsuru/config"
	"github.com/tsuru/docker-cluster/cluster"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/provision/docker/container"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var digestRegexp = regexp.MustCompile(`(?m)^Digest: (.*)$`)

type DockerProvisioner interface {
	Cluster() *cluster.Cluster
	RegistryAuthConfig() docker.AuthConfiguration
}

const (
	bsUniqueID         = "bs"
	bsDefaultImageName = "tsuru/bs:v1"
)

type Env struct {
	Name  string
	Value string
}

type PoolEnvs struct {
	Name string
	Envs []Env
}

type Config struct {
	ID    string `bson:"_id"`
	Image string
	Token string
	Envs  []Env
	Pools []PoolEnvs
}

type EnvMap map[string]string

type PoolEnvMap map[string]EnvMap

func (conf *Config) UpdateEnvMaps(envMap EnvMap, poolEnvMap PoolEnvMap) error {
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
			poolEnvMap[p.Name] = make(EnvMap)
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

func (conf *Config) getImage() string {
	if conf != nil && conf.Image != "" {
		return conf.Image
	}
	bsImage, _ := config.GetString("docker:bs:image")
	if bsImage == "" {
		bsImage = bsDefaultImageName
	}
	return bsImage
}

func (conf *Config) EnvListForEndpoint(dockerEndpoint, poolName string) ([]string, error) {
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
		"SYSLOG_LISTEN_ADDRESS=udp://0.0.0.0:" + strconv.Itoa(container.BsSysLogPort()),
	}
	envMap := EnvMap{}
	poolEnvMap := PoolEnvMap{}
	err = conf.UpdateEnvMaps(envMap, poolEnvMap)
	if err != nil {
		return nil, err
	}
	for envName, envValue := range envMap {
		envList = append(envList, fmt.Sprintf("%s=%s", envName, envValue))
	}
	for envName, envValue := range poolEnvMap[poolName] {
		envList = append(envList, fmt.Sprintf("%s=%s", envName, envValue))
	}
	return envList, nil
}

func (conf *Config) getToken() (string, error) {
	if conf.Token != "" {
		return conf.Token, nil
	}
	coll, err := collection()
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

func (conf *Config) EnvValueForPool(envName, poolName string) string {
	for _, poolEnvs := range conf.Pools {
		if poolEnvs.Name == poolName {
			for _, env := range poolEnvs.Envs {
				if env.Name == envName {
					return env.Value
				}
			}
		}
	}
	for _, env := range conf.Envs {
		if env.Name == envName {
			return env.Value
		}
	}
	return ""
}

func bsConfigFromEnvMaps(envMap EnvMap, poolEnvMap PoolEnvMap) *Config {
	var finalConf Config
	for name, value := range envMap {
		finalConf.Envs = append(finalConf.Envs, Env{Name: name, Value: value})
	}
	for poolName, envMap := range poolEnvMap {
		poolEnv := PoolEnvs{Name: poolName}
		for name, value := range envMap {
			poolEnv.Envs = append(poolEnv.Envs, Env{Name: name, Value: value})
		}
		finalConf.Pools = append(finalConf.Pools, poolEnv)
	}
	return &finalConf
}

func SaveImage(digest string) error {
	coll, err := collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	_, err = coll.UpsertId(bsUniqueID, bson.M{"$set": bson.M{"image": digest}})
	return err
}

func SaveEnvs(envMap EnvMap, poolEnvMap PoolEnvMap) error {
	finalConf := bsConfigFromEnvMaps(envMap, poolEnvMap)
	coll, err := collection()
	if err != nil {
		return err
	}
	defer coll.Close()
	_, err = coll.UpsertId(bsUniqueID, bson.M{"$set": bson.M{"envs": finalConf.Envs, "pools": finalConf.Pools}})
	return err
}

func LoadConfig(pools []string) (*Config, error) {
	var config Config
	coll, err := collection()
	if err != nil {
		return nil, err
	}
	defer coll.Close()
	err = coll.FindId(bsUniqueID).One(&config)
	if err != nil {
		return nil, err
	}
	if pools != nil {
		poolEnvs := make([]PoolEnvs, 0, len(pools))
		for _, pool := range pools {
			for _, poolEnv := range config.Pools {
				if poolEnv.Name == pool {
					poolEnvs = append(poolEnvs, poolEnv)
					break
				}
			}
		}
		config.Pools = poolEnvs
	}
	return &config, nil
}

func collection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	return conn.Collection("bsconfig"), nil
}

func dockerClient(endpoint string) (*docker.Client, error) {
	client, err := docker.NewClient(endpoint)
	if err != nil {
		return nil, err
	}
	client.HTTPClient = net.Dial5Full300Client
	client.Dialer = net.Dial5Dialer
	return client, nil
}

func createContainer(dockerEndpoint, poolName string, p DockerProvisioner, relaunch bool) error {
	client, err := dockerClient(dockerEndpoint)
	if err != nil {
		return err
	}
	bsConf, err := LoadConfig(nil)
	if err != nil {
		if err != mgo.ErrNotFound {
			return err
		}
		bsConf = &Config{}
	}
	bsImage := bsConf.getImage()
	err = pullBsImage(bsImage, dockerEndpoint, p)
	if err != nil {
		return err
	}
	hostConfig := docker.HostConfig{
		RestartPolicy: docker.AlwaysRestart(),
		Privileged:    true,
		NetworkMode:   "host",
	}
	socket, _ := config.GetString("docker:bs:socket")
	if socket != "" {
		hostConfig.Binds = []string{fmt.Sprintf("%s:/var/run/docker.sock:rw", socket)}
	}
	env, err := bsConf.EnvListForEndpoint(dockerEndpoint, poolName)
	if err != nil {
		return err
	}
	opts := docker.CreateContainerOptions{
		Name:       "big-sibling",
		HostConfig: &hostConfig,
		Config: &docker.Config{
			Image: bsImage,
			Env:   env,
		},
	}
	container, err := client.CreateContainer(opts)
	if relaunch && err == docker.ErrContainerAlreadyExists {
		err = client.RemoveContainer(docker.RemoveContainerOptions{ID: opts.Name, Force: true})
		if err != nil {
			return err
		}
		container, err = client.CreateContainer(opts)
	}
	if err != nil && err != docker.ErrContainerAlreadyExists {
		return err
	}
	if container == nil {
		container, err = client.InspectContainer("big-sibling")
		if err != nil {
			return err
		}
	}
	err = client.StartContainer(container.ID, &hostConfig)
	if _, ok := err.(*docker.ContainerAlreadyRunning); !ok {
		return err
	}
	return nil
}

func pullWithRetry(maxTries int, image, dockerEndpoint string, p DockerProvisioner) (string, error) {
	client, err := dockerClient(dockerEndpoint)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	pullOpts := docker.PullImageOptions{Repository: image, OutputStream: &buf}
	registryAuth := p.RegistryAuthConfig()
	for ; maxTries > 0; maxTries-- {
		err = client.PullImage(pullOpts, registryAuth)
		if err == nil {
			return buf.String(), nil
		}
	}
	return "", err
}

func pullBsImage(image, dockerEndpoint string, p DockerProvisioner) error {
	output, err := pullWithRetry(3, image, dockerEndpoint, p)
	if err != nil {
		return err
	}
	if shouldPinBsImage(image) {
		match := digestRegexp.FindAllStringSubmatch(output, 1)
		if len(match) > 0 {
			image += "@" + match[0][1]
		}
	}
	return SaveImage(image)
}

func shouldPinBsImage(image string) bool {
	parts := strings.SplitN(image, "/", 3)
	lastPart := parts[len(parts)-1]
	return len(strings.SplitN(lastPart, ":", 2)) < 2
}

// RecreateContainers relaunch all bs containers in the cluster for the given
// DockerProvisioner, logging progress to the given writer.
//
// It assumes that the given writer is thread safe.
func RecreateContainers(p DockerProvisioner, w io.Writer) error {
	cluster := p.Cluster()
	nodes, err := cluster.UnfilteredNodes()
	if err != nil {
		return err
	}
	errChan := make(chan error, len(nodes))
	wg := sync.WaitGroup{}
	log.Debugf("[bs containers] recreating %d containers", len(nodes))
	for i := range nodes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			node := &nodes[i]
			pool := node.Metadata["pool"]
			log.Debugf("[bs containers] recreating container in %s [%s]", node.Address, pool)
			fmt.Fprintf(w, "relaunching bs container in the node %s [%s]\n", node.Address, pool)
			err := createContainer(node.Address, pool, p, true)
			if err != nil {
				msg := fmt.Sprintf("[bs containers] failed to create container in %s [%s]: %s", node.Address, pool, err)
				log.Error(msg)
				err = errors.New(msg)
				errChan <- err
			}
		}(i)
	}
	wg.Wait()
	close(errChan)
	return <-errChan
}

type ClusterHook struct {
	Provisioner DockerProvisioner
}

func (h *ClusterHook) BeforeCreateContainer(node cluster.Node) error {
	err := createContainer(node.Address, node.Metadata["pool"], h.Provisioner, false)
	if err != nil {
		return err
	}
	return nil
}
