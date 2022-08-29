// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"errors"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	apptypes "github.com/tsuru/tsuru/types/app"
)

var _ apptypes.AppServiceEnvVarStorage = (*appServiceEnvVarStorage)(nil)

type appServiceEnvVarStorage struct{}

func (s *appServiceEnvVarStorage) FindAll(ctx context.Context, a apptypes.App) ([]apptypes.ServiceEnvVar, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	var app struct{ ServiceEnvs []apptypes.ServiceEnvVar }
	err = conn.Apps().Find(bson.M{"name": a.GetName()}).One(&app)
	if err != nil {
		return nil, err
	}

	return app.ServiceEnvs, nil
}

func (s *appServiceEnvVarStorage) Remove(ctx context.Context, a apptypes.App, envs []apptypes.ServiceEnvVarIdentifier) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(envs) == 0 {
		return nil
	}

	svcEnvs, err := s.FindAll(ctx, a)
	if err != nil {
		return err
	}

	for _, env := range envs {
		index, found := findServiceEnvVar(svcEnvs, env.GetServiceName(), env.GetInstanceName(), env.GetEnvVarName())
		if !found {
			return errors.New("service env var not found")
		}

		svcEnvs = append(svcEnvs[:index], svcEnvs[index+1:]...)
	}

	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()

	return conn.Apps().Update(bson.M{"name": a.GetName()}, bson.M{"$set": bson.M{"serviceenvs": svcEnvs}})
}

func (s *appServiceEnvVarStorage) Upsert(ctx context.Context, a apptypes.App, envs []apptypes.ServiceEnvVar) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()

	svcEnvs, err := s.FindAll(ctx, a)
	if err != nil {
		return err
	}

	for _, env := range envs {
		index, found := findServiceEnvVar(svcEnvs, env.ServiceName, env.InstanceName, env.Name)
		if found {
			svcEnvs[index] = env
			continue
		}

		svcEnvs = append(svcEnvs, env)
	}

	return conn.Apps().Update(bson.M{"name": a.GetName()}, bson.M{"$set": bson.M{"serviceenvs": svcEnvs}})
}

func findServiceEnvVar(svcEnvs []apptypes.ServiceEnvVar, service, instance, envName string) (int, bool) {
	for i, e := range svcEnvs {
		if e.ServiceName == service && e.InstanceName == instance && e.Name == envName {
			return i, true
		}
	}

	return -1, false
}
