// Copyright 2016 tsuru authors. All rights reserved.
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
	ErrRoleEventNotFound = errors.New("role event not found")

	RoleEventUserCreate = &RoleEvent{
		name:        "user-create",
		context:     CtxGlobal,
		Description: "role added to user when user is created",
	}
	RoleEventTeamCreate = &RoleEvent{
		name:        "team-create",
		context:     CtxTeam,
		Description: "role added to user when a new team is created",
	}

	RoleEventMap = map[string]*RoleEvent{
		RoleEventUserCreate.name: RoleEventUserCreate,
		RoleEventTeamCreate.name: RoleEventTeamCreate,
	}
)

type ErrRoleEventWrongContext struct {
	expected string
	role     string
}

func (e ErrRoleEventWrongContext) Error() string {
	return fmt.Sprintf("wrong context type for role event, expected %q role has %q", e.expected, e.role)
}

type RoleEvent struct {
	name        string
	context     contextType
	Description string
}

func (e *RoleEvent) String() string {
	return e.name
}

type Role struct {
	Name        string      `bson:"_id" json:"name"`
	ContextType contextType `json:"context"`
	SchemeNames []string    `json:"scheme_names,omitempty"`
	Events      []string    `json:"events,omitempty"`
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
	if err != nil {
		return nil, err
	}
	for i := range roles {
		roles[i].filterValidSchemes()
	}
	return roles, nil
}

func ListRolesWithEvents() ([]Role, error) {
	var roles []Role
	coll, err := rolesCollection()
	if err != nil {
		return roles, err
	}
	defer coll.Close()
	err = coll.Find(bson.M{"events": bson.M{"$not": bson.M{"$size": 0}, "$exists": true}}).All(&roles)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		roles[i].filterValidSchemes()
	}
	return roles, nil
}

func ListRolesForEvent(evt *RoleEvent) ([]Role, error) {
	if evt == nil {
		return nil, errors.New("invalid role event")
	}
	var roles []Role
	coll, err := rolesCollection()
	if err != nil {
		return roles, err
	}
	defer coll.Close()
	err = coll.Find(bson.M{"events": evt.name}).All(&roles)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		roles[i].filterValidSchemes()
	}
	return roles, nil
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
	if err != nil {
		return role, err
	}
	role.filterValidSchemes()
	return role, nil
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
		if permName == "" {
			return fmt.Errorf("empty permission name")
		}
		if permName == "*" {
			permName = ""
		}
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

func (r *Role) filterValidSchemes() PermissionSchemeList {
	schemes := make(PermissionSchemeList, 0, len(r.SchemeNames))
	sort.Strings(r.SchemeNames)
	for i := 0; i < len(r.SchemeNames); i++ {
		schemeName := r.SchemeNames[i]
		if schemeName == "*" {
			schemeName = ""
		}
		scheme := PermissionRegistry.getSubRegistry(schemeName)
		if scheme == nil {
			// permission schemes might be removed or renamed, invalid entries
			// in the database shouldn't be a problem.
			r.SchemeNames = append(r.SchemeNames[:i], r.SchemeNames[i+1:]...)
			i--
			continue
		}
		schemes = append(schemes, &scheme.PermissionScheme)
	}
	return schemes
}

func (r *Role) PermissionsFor(contextValue string) []Permission {
	schemes := r.filterValidSchemes()
	permissions := make([]Permission, len(schemes))
	for i, scheme := range schemes {
		permissions[i] = Permission{
			Scheme: scheme,
			Context: PermissionContext{
				CtxType: r.ContextType,
				Value:   contextValue,
			},
		}
	}
	return permissions
}

func (r *Role) AddEvent(eventName string) error {
	roleEvent := RoleEventMap[eventName]
	if roleEvent == nil {
		return ErrRoleEventNotFound
	}
	if r.ContextType != roleEvent.context {
		return ErrRoleEventWrongContext{expected: string(roleEvent.context), role: string(r.ContextType)}
	}
	coll, err := rolesCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.UpdateId(r.Name, bson.M{"$addToSet": bson.M{"events": eventName}})
	if err != nil {
		return err
	}
	dbRole, err := FindRole(r.Name)
	if err != nil {
		return err
	}
	r.Events = dbRole.Events
	return nil
}

func (r *Role) RemoveEvent(eventName string) error {
	coll, err := rolesCollection()
	if err != nil {
		return err
	}
	defer coll.Close()
	err = coll.UpdateId(r.Name, bson.M{"$pull": bson.M{"events": eventName}})
	if err != nil {
		return err
	}
	dbRole, err := FindRole(r.Name)
	if err != nil {
		return err
	}
	r.Events = dbRole.Events
	return nil
}

func rolesCollection() (*storage.Collection, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	return conn.Roles(), nil
}
