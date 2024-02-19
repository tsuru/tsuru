// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permissiontest

import (
	"context"
	"strings"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/types/quota"
	check "gopkg.in/check.v1"
)

func CustomUserWithPermission(c *check.C, scheme auth.Scheme, baseName string, perm ...permission.Permission) (*auth.User, auth.Token) {
	user := &auth.User{Email: baseName + "@groundcontrol.com", Password: "123456", Quota: quota.UnlimitedQuota}
	_, err := scheme.(auth.UserScheme).Create(context.TODO(), user)
	c.Assert(err, check.IsNil)
	return user, ExistingUserWithPermission(c, scheme, user, perm...)
}

func ExistingUserWithPermission(c *check.C, scheme auth.Scheme, user *auth.User, perm ...permission.Permission) auth.Token {
	token, err := scheme.(auth.UserScheme).Login(context.TODO(), map[string]string{"email": user.Email, "password": "123456"})
	c.Assert(err, check.IsNil)
	for _, p := range perm {
		baseName := user.Email
		idx := strings.Index(baseName, "@")
		if idx != -1 {
			baseName = baseName[:idx]
		}
		role, err := permission.NewRole(baseName+p.Scheme.FullName()+p.Context.Value, string(p.Context.CtxType), "")
		c.Assert(err, check.IsNil)
		name := p.Scheme.FullName()
		if name == "" {
			name = "*"
		}
		err = role.AddPermissions(name)
		c.Assert(err, check.IsNil)
		err = user.AddRole(role.Name, p.Context.Value)
		c.Assert(err, check.IsNil)
	}
	return token
}
