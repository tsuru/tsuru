// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"errors"
	"fmt"
	"strings"
)

type User struct {
	Quota    *AuthQuota
	Email    string
	Password string
	APIKey   string
	Roles    []RoleInstance
}

type RoleInstance struct {
	Name         string
	ContextValue string
}

type ErrTeamStillUsed struct {
	Apps             []string
	ServiceInstances []string
}

var (
	ErrUserNotFound = errors.New("user not found")
	ErrInvalidKey   = errors.New("invalid key")
	ErrKeyDisabled  = errors.New("key management is disabled")
)

func (e *ErrTeamStillUsed) Error() string {
	if len(e.Apps) > 0 {
		return fmt.Sprintf("Apps: %s", strings.Join(e.Apps, ", "))
	}
	return fmt.Sprintf("Service instances: %s", strings.Join(e.ServiceInstances, ", "))
}
