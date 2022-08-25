// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"sort"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	apptypes "github.com/tsuru/tsuru/types/app"
)

var _ apptypes.AppEnvVarStorage = &appEnvVarStorage{}

type appEnvVarStorage struct{}

func (s *appEnvVarStorage) ListAppEnvs(ctx context.Context, appName string) ([]apptypes.EnvVar, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var app struct{ Env map[string]apptypes.EnvVar }
	err = conn.Apps().Find(bson.M{"name": appName}).One(&app)
	if err != nil {
		return nil, err
	}
	envs := make([]apptypes.EnvVar, 0, len(app.Env))
	for _, ev := range app.Env {
		envs = append(envs, ev)
	}
	sort.Slice(envs, func(i, j int) bool { return envs[i].Name < envs[j].Name })
	return envs, nil
}

func (s *appEnvVarStorage) ListServiceEnvs(ctx context.Context, appName string) ([]apptypes.ServiceEnvVar, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var app struct{ ServiceEnvs []apptypes.ServiceEnvVar }
	err = conn.Apps().Find(bson.M{"name": appName}).One(&app)
	if err != nil {
		return nil, err
	}
	sort.Slice(app.ServiceEnvs, func(i, j int) bool { return app.ServiceEnvs[i].Name < app.ServiceEnvs[j].Name })
	return app.ServiceEnvs, nil
}
