// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	stdContext "context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	_ "github.com/tsuru/tsuru/auth/native"
	_ "github.com/tsuru/tsuru/auth/oauth"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/permission"
	_ "github.com/tsuru/tsuru/storage/mongodb"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

type createRootUserCmd struct{}

func (createRootUserCmd) Run(context *cmd.Context) error {
	context.RawOutput()
	scheme, err := config.GetString("auth:scheme")
	if err != nil {
		scheme = nativeSchemeName
	}
	app.AuthScheme, err = auth.GetScheme(scheme)
	if err != nil {
		return err
	}
	email := context.Args[0]
	user, err := auth.GetUserByEmail(email)
	if err == nil {
		err = addSuperRole(user)
		if err != nil {
			return err
		}
		fmt.Fprintln(context.Stdout, "Root user successfully updated.")
	}
	var confirm, password string
	if scheme == nativeSchemeName {
		fmt.Fprint(context.Stdout, "Password: ")
		password, err = cmd.PasswordFromReader(context.Stdin)
		if err != nil {
			return err
		}
		fmt.Fprint(context.Stdout, "\nConfirm: ")
		confirm, err = cmd.PasswordFromReader(context.Stdin)
		if err != nil {
			return err
		}
		fmt.Fprintln(context.Stdout)
		if password != confirm {
			return errors.New("Passwords didn't match.")
		}
	}

	if userScheme, ok := app.AuthScheme.(auth.UserScheme); ok {
		user, err = userScheme.Create(stdContext.Background(), &auth.User{
			Email:    email,
			Password: password,
		})
		if err != nil {
			return err
		}
	} else {
		err = user.Create()
		if err != nil {
			return err
		}
	}

	err = addSuperRole(user)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "Root user successfully created.")

	return nil
}

func addSuperRole(u *auth.User) error {
	defaultRoleName := "AllowAll"
	r, err := permission.FindRole(defaultRoleName)
	if err != nil {
		r, err = permission.NewRole(defaultRoleName, string(permTypes.CtxGlobal), "")
		if err != nil {
			return err
		}
	}
	err = r.AddPermissions("*")
	if err != nil {
		return err
	}
	return u.AddRole(defaultRoleName, "")
}

func (createRootUserCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:  "root-user-create",
		Usage: "root-user-create <email>",
		Desc: `Create a root user with all permission. This user can be used when
bootstraping a tsuru cloud. It can be erased after other users are created and
roles are properly created and assigned.`,
		MinArgs: 1,
	}
}
