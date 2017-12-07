// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pool

import (
	"github.com/tsuru/tsuru/db"
)

type poolWithTeams struct {
	Name   string `bson:"_id"`
	Teams  []string
	Public bool
}

func MigratePoolTeamsToPoolConstraints() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var pools []poolWithTeams
	err = conn.Pools().Find(nil).All(&pools)
	if err != nil {
		return err
	}
	for _, p := range pools {
		values := []string{"*"}
		if !p.Public {
			values = p.Teams
		}
		err := SetPoolConstraint(&PoolConstraint{PoolExpr: p.Name, Field: ConstraintTypeTeam, Values: values})
		if err != nil {
			return err
		}
	}
	return nil
}
