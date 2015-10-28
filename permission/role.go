// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/storage"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	ErrRoleNotFound      = errors.New("role not found")
	ErrRoleAlreadyExists = errors.New("role already exists")
)

type Role struct {
	Name        string      `bson:"_id" json:"name"`
	ContextType contextType `json:"context"`
	SchemeNames []string    `json:"scheme_names,omitempty"`
}

func NewRole(name string, ctx string) (Role, error) {
	ctxType, err := parseContext(ctx)
	if err != nil {
		return Role{}, err
	}
	name = strings.TrimSpace(name)
	if len(name) == 0 {
		return Role{}, fmt.Errorf("invalid role name %q", name)
	}
	coll, err := rolesCollection()
	if err != nil {
		return Role{}, err
	}
	defer coll.Close()
	role := Role{Name: name, ContextType: ctxType}
	err = coll.Insert(role)
	if mgo.IsDup(err) {
		return Role{}, ErrRoleAlreadyExists
	}
	return role, err
}

func ListRoles() ([]Role, error) {
	var roles []Role
	coll, err := rolesCollection()
	if err != nil {
		return roles, err
	}
	defer coll.Close()
	err = coll.Find(nil).All(&roles)
	return roles, err
}

func FindRole(name string) (Role, error) {
	var role Role
	coll, err := rolesCollection()
	if err != nil {
		return role, err
	}
	defer coll.Close()
	err = coll.FindId(name).One(&role)
	if err == mgo.ErrNotFound {
		return role, ErrRoleNotFound
	}
	return role, err
}

func DestroyRole(name string) error {
	coll, err := rolesCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.RemoveId(name)
	if err == mgo.ErrNotFound {
		return ErrRoleNotFound
	}
	return err
}

func (r *Role) AddPermissions(permNames ...string) error {
	for _, permName := range permNames {
		reg := PermissionRegistry.getSubRegistry(permName)
		if reg == nil {
			return fmt.Errorf("permission named %q not found", permName)
		}
		var found bool
		for _, ctxType := range reg.AllowedContexts() {
			if ctxType == r.ContextType {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("permission %q not allowed with context of type %q", permName, r.ContextType)
		}
	}
	coll, err := rolesCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.UpdateId(r.Name, bson.M{"$addToSet": bson.M{"schemenames": bson.M{"$each": permNames}}})
	if err != nil {
		return err
	}
	dbRole, err := FindRole(r.Name)
	if err != nil {
		return err
	}
	r.SchemeNames = dbRole.SchemeNames
	return nil
}

func (r *Role) RemovePermissions(permNames ...string) error {
	coll, err := rolesCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.UpdateId(r.Name, bson.M{"$pullAll": bson.M{"schemenames": permNames}})
	if err != nil {
		return err
	}
	dbRole, err := FindRole(r.Name)
	if err != nil {
		return err
	}
	r.SchemeNames = dbRole.SchemeNames
	return nil
}

func (r *Role) PermisionsFor(contextValue string) []Permission {
	permissions := make([]Permission, len(r.SchemeNames))
	sort.Strings(r.SchemeNames)
	for i, schemeName := range r.SchemeNames {
		scheme := PermissionRegistry.getSubRegistry(schemeName)
		permissions[i] = Permission{
			Scheme: &scheme.permissionScheme,
			Context: permissionContext{
				CtxType: r.ContextType,
				Value:   contextValue,
			},
		}
	}
	return permissions
}

func rolesCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	return conn.Roles(), nil
}
