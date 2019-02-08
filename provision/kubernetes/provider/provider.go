// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	rcluster "github.com/rancher/kontainer-engine/cluster"
	"github.com/rancher/kontainer-engine/service"
	"github.com/rancher/kontainer-engine/types"
	"github.com/rancher/rke/log"
	v3 "github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/tsuru/tsuru/provision/cluster"
	provTypes "github.com/tsuru/tsuru/types/provision"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kontainerDataCreateClusterKey    = "kontainer-data"
	kontainerDeletedCreateClusterKey = "kontainer-deleted"
	driverCreateClusterKey           = "driver"

	driverNameKontainerKey = "driverName"

	tokenClusterKey    = "token"
	userClusterKey     = "username"
	passwordClusterKey = "password"
)

type clusterStorage struct {
	storage provTypes.ClusterStorage
}

func (s *clusterStorage) GetStatus(name string) (string, error) {
	c, err := s.Get(name)
	if err != nil {
		return "", err
	}
	return c.Status, nil
}

func (s *clusterStorage) Get(name string) (rcluster.Cluster, error) {
	var c rcluster.Cluster
	tc, err := s.storage.FindByName(name)
	if err != nil {
		return c, errors.WithStack(err)
	}
	if tc.CreateData != nil {
		if _, ok := tc.CreateData[kontainerDeletedCreateClusterKey]; ok {
			return c, provTypes.ErrClusterNotFound
		}
		if data, ok := tc.CreateData[kontainerDataCreateClusterKey]; ok {
			err = json.Unmarshal([]byte(data), &c)
			if err != nil {
				return c, errors.WithStack(err)
			}
		}
	}
	c.Name = tc.Name
	return c, nil
}

func (s *clusterStorage) Remove(name string) error {
	tCluster, err := s.storage.FindByName(name)
	if err != nil {
		return err
	}
	if tCluster.CreateData == nil {
		tCluster.CreateData = map[string]string{}
	}
	tCluster.CreateData[kontainerDeletedCreateClusterKey] = "true"
	return errors.WithStack(s.storage.Upsert(*tCluster))
}

func (s *clusterStorage) Store(c rcluster.Cluster) error {
	tc, err := s.storage.FindByName(c.Name)
	if err != nil {
		return errors.WithStack(err)
	}
	tc.Addresses = []string{c.Endpoint}
	tc.CaCert, err = base64.StdEncoding.DecodeString(c.RootCACert)
	if err != nil {
		return errors.WithStack(err)
	}
	tc.ClientKey, err = base64.StdEncoding.DecodeString(c.ClientKey)
	if err != nil {
		return errors.WithStack(err)
	}
	tc.ClientCert, err = base64.StdEncoding.DecodeString(c.ClientCertificate)
	if err != nil {
		return errors.WithStack(err)
	}
	if tc.CustomData == nil {
		tc.CustomData = make(map[string]string)
	}
	if c.Username != "" {
		tc.CustomData[userClusterKey] = c.Username
	}
	if c.Password != "" {
		tc.CustomData[passwordClusterKey] = c.Password
	}
	if c.ServiceAccountToken != "" {
		tc.CustomData[tokenClusterKey] = c.ServiceAccountToken
	}
	serialized, err := json.Marshal(c)
	if err != nil {
		return errors.WithStack(err)
	}
	if tc.CreateData == nil {
		tc.CreateData = make(map[string]string)
	}
	tc.CreateData[kontainerDataCreateClusterKey] = string(serialized)
	delete(tc.CreateData, kontainerDeletedCreateClusterKey)
	return errors.WithStack(s.storage.Upsert(*tc))
}

func (s *clusterStorage) PersistStatus(c rcluster.Cluster, status string) error {
	var err error
	c, err = s.Get(c.Name)
	if err != nil {
		return err
	}
	c.Status = status
	return s.Store(c)
}

func getKontainerDriver(driver string) *v3.KontainerDriver {
	return &v3.KontainerDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name: driver,
		},
		Spec: v3.KontainerDriverSpec{
			BuiltIn: true,
		},
	}
}

func baseClusterSpec(driver string) v3.ClusterSpec {
	return v3.ClusterSpec{
		GenericEngineConfig: &v3.MapStringInterface{
			driverNameKontainerKey: driver,
		},
	}
}

func setFlagsToCluster(config v3.MapStringInterface, flags *types.DriverFlags, customData map[string]string) error {
	for k, v := range flags.Options {
		raw, ok := customData[k]
		if !ok {
			continue
		}
		switch v.Type {
		case types.IntType:
			val, _ := strconv.Atoi(raw)
			config[k] = val
		case types.IntPointerType:
			val, _ := strconv.Atoi(raw)
			config[k] = &val
		case types.StringType:
			config[k] = raw
		case types.StringSliceType:
			parts := strings.Split(raw, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			config[k] = parts
		case types.BoolType:
			val, _ := strconv.ParseBool(raw)
			config[k] = val
		case types.BoolPointerType:
			val, _ := strconv.ParseBool(raw)
			config[k] = &val
		default:
			return errors.Errorf("unknown type for flag %v: %v", k, v.Type)
		}
	}
	return nil
}

type writerLogger struct {
	w io.Writer
}

func (l *writerLogger) Infof(msg string, args ...interface{}) {
	msg = fmt.Sprintf("[info] %s", msg)
	fmt.Fprintf(l.w, msg, args...)
}
func (l *writerLogger) Warnf(msg string, args ...interface{}) {
	msg = fmt.Sprintf("[warn] %s", msg)
	fmt.Fprintf(l.w, msg, args...)
}

type engineData struct {
	engine service.EngineService
	driver *v3.KontainerDriver
	spec   v3.ClusterSpec
}

func prepareEngine(ctx context.Context, name string, customData map[string]string, w io.Writer) (engineData, error) {
	if w != nil {
		log.SetLogger(ctx, &writerLogger{w: w})
	}
	stor, err := cluster.ClusterStorage()
	if err != nil {
		return engineData{}, errors.WithStack(err)
	}
	engine := service.NewEngineService(&clusterStorage{
		storage: stor,
	})
	driverName := customData[driverCreateClusterKey]
	clusterSpec := baseClusterSpec(driverName)
	driver := getKontainerDriver(driverName)
	flags, err := engine.GetDriverCreateOptions(ctx, name, driver, clusterSpec)
	if err != nil {
		return engineData{}, errors.WithStack(err)
	}
	err = setFlagsToCluster(*(clusterSpec.GenericEngineConfig), flags, customData)
	if err != nil {
		return engineData{}, err
	}
	return engineData{
		engine: engine,
		driver: driver,
		spec:   clusterSpec,
	}, nil
}

func FormattedCreateOptions() (map[string]string, error) {
	result := make(map[string]string)
	var driverNames []string
	for driverName, driver := range service.Drivers {
		driverNames = append(driverNames, driverName)
		flags, err := driver.GetDriverCreateOptions(context.Background())
		if err != nil {
			return nil, errors.WithStack(err)
		}
		for flagName, flag := range flags.Options {
			key := fmt.Sprintf("%v - %v", driverName, flagName)
			value := fmt.Sprintf("%s (type: %s)", flag.Usage, flag.Type)
			result[key] = value
		}
	}
	result[driverCreateClusterKey] = fmt.Sprintf("Cluster driver being used, available: %v", strings.Join(driverNames, ", "))
	return result, nil
}

func CreateCluster(ctx context.Context, name string, customData map[string]string, w io.Writer) error {
	engineData, err := prepareEngine(ctx, name, customData, w)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Creating cluster %q using driver %q\n", name, engineData.driver.Name)
	_, _, _, err = engineData.engine.Create(ctx, name, engineData.driver, engineData.spec)
	return errors.WithStack(err)
}

func UpdateCluster(ctx context.Context, name string, customData map[string]string, w io.Writer) error {
	engineData, err := prepareEngine(ctx, name, customData, w)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Updating cluster %q using driver %q\n", name, engineData.driver.Name)
	_, _, _, err = engineData.engine.Update(ctx, name, engineData.driver, engineData.spec)
	return errors.WithStack(err)
}

func DeleteCluster(ctx context.Context, name string, customData map[string]string, w io.Writer) error {
	engineData, err := prepareEngine(ctx, name, customData, w)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Deleting cluster %q using driver %q\n", name, engineData.driver.Name)
	return errors.WithStack(engineData.engine.Remove(ctx, name, engineData.driver, engineData.spec))
}
