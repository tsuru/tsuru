// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/permission"
	authTypes "github.com/tsuru/tsuru/types/auth"
	permTypes "github.com/tsuru/tsuru/types/permission"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	check "gopkg.in/check.v1"
)

func (s *S) TestParseToken(c *check.C) {
	t, err := ParseToken("type token")
	c.Assert(err, check.IsNil)
	c.Assert(t, check.Equals, "token")
	t, err = ParseToken("token")
	c.Assert(err, check.IsNil)
	c.Assert(t, check.Equals, "token")
	t, err = ParseToken("type ble ble")
	c.Assert(err, check.Equals, ErrInvalidToken)
	c.Assert(t, check.Equals, "")
	t, err = ParseToken("")
	c.Assert(err, check.Equals, ErrInvalidToken)
	c.Assert(t, check.Equals, "")
}

type dynamicPermissionToken struct {
	user *authTypes.User
	err  error
}

func (t *dynamicPermissionToken) GetValue() string {
	return ""
}

func (t *dynamicPermissionToken) GetUserName() string {
	return ""
}

func (t *dynamicPermissionToken) User(ctx context.Context) (*authTypes.User, error) {
	return t.user, t.err
}

func (t *dynamicPermissionToken) Engine() string {
	return "test"
}

func (t *dynamicPermissionToken) Permissions(ctx context.Context) ([]permTypes.Permission, error) {
	return nil, nil
}

func (s *S) TestBaseTokenDynamicPermission(c *check.C) {
	createServiceWithDynamicAction(c, "acl", "rules.sync")
	role, err := permission.NewRole(context.TODO(), "dynamic-role-base-token", "team", "")
	c.Assert(err, check.IsNil)
	err = role.AddDynamicPermissions(context.TODO(), "service-action.acl.rules.sync")
	c.Assert(err, check.IsNil)

	token := &dynamicPermissionToken{
		user: &authTypes.User{
			Email: "base-token@tsuru.io",
			Roles: []authTypes.RoleInstance{{Name: role.Name, ContextValue: "team-1"}},
		},
	}
	perms, err := BaseTokenDynamicPermission(context.TODO(), token)
	c.Assert(err, check.IsNil)
	c.Assert(perms, check.DeepEquals, []permTypes.Permission{{
		Scheme:  mustNewDynamic(c, "service-action.acl.rules.sync"),
		Context: permission.Context(permTypes.CtxTeam, "team-1"),
	}})
}

func (s *S) TestBaseTokenDynamicPermissionUserError(c *check.C) {
	_, err := BaseTokenDynamicPermission(context.TODO(), &dynamicPermissionToken{err: errors.New("boom")})
	c.Assert(err, check.ErrorMatches, "boom")
}

func mustNewDynamic(c *check.C, name string) *permTypes.PermissionScheme {
	scheme, ok := permission.NewDynamic(name)
	c.Assert(ok, check.Equals, true)
	return scheme
}

func createServiceWithDynamicAction(c *check.C, serviceName string, actions ...string) {
	servicesCollection, err := storagev2.ServicesCollection()
	c.Assert(err, check.IsNil)
	operations := make([]mongoBSON.M, len(actions))
	for i, action := range actions {
		operations[i] = mongoBSON.M{"method": "POST", "path": "/sync", "action": action}
	}
	_, err = servicesCollection.InsertOne(context.TODO(), mongoBSON.M{
		"_id":      serviceName,
		"manifest": mongoBSON.M{"enabled": true, "operations": operations},
	})
	c.Assert(err, check.IsNil)
}
