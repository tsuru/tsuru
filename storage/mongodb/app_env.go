// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"fmt"
	"sort"

	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	apptypes "github.com/tsuru/tsuru/types/app"
)

var _ apptypes.AppEnvVarStorage = (*appEnvVarStorage)(nil)

type appEnvVarStorage struct{}

func (s *appEnvVarStorage) FindAll(ctx context.Context, a apptypes.App) ([]apptypes.EnvVar, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	var app struct{ Env map[string]apptypes.EnvVar }
	err = conn.Apps().Find(bson.M{"name": a.GetName()}).One(&app)
	if err != nil {
		return nil, err
	}

	envs := make([]apptypes.EnvVar, 0, len(app.Env))
	for _, ev := range app.Env {
		envs = append(envs, ev)
	}

	// NOTE: since env vars are stored as map, we should ensure the order for testing purposes.
	sort.Slice(envs, func(i, j int) bool { return envs[i].Name < envs[j].Name })

	return envs, nil
}

func (s *appEnvVarStorage) Upsert(ctx context.Context, a apptypes.App, envs []apptypes.EnvVar) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	existingEnvs, err := s.FindAll(ctx, a)
	if err != nil {
		return err
	}

	existingEnvMap, updatedEnvMap := envVarsToMap(existingEnvs), envVarsToMap(envs)

	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()

	return conn.Apps().Update(bson.M{"name": a.GetName()}, bson.M{"$set": bson.M{"env": mergeEnvVars(existingEnvMap, updatedEnvMap)}})
}

func (s *appEnvVarStorage) Remove(ctx context.Context, a apptypes.App, envs []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(envs) == 0 {
		return nil
	}

	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()

	fieldsToRemove := bson.M{}
	for _, env := range envs {
		fieldsToRemove[fmt.Sprintf("env.%s", env)] = ""
	}

	return conn.Apps().Update(bson.M{"name": a.GetName()}, bson.M{"$unset": fieldsToRemove})
}

func envVarsToMap(envs []apptypes.EnvVar) map[string]apptypes.EnvVar {
	envMap := make(map[string]apptypes.EnvVar)
	for _, env := range envs {
		envMap[env.Name] = env
	}
	return envMap
}

func mergeEnvVars(base, override map[string]apptypes.EnvVar) map[string]apptypes.EnvVar {
	merged := make(map[string]apptypes.EnvVar)
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range override {
		merged[k] = v
	}
	return merged
}
