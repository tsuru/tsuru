// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tsuru/tsuru/types/quota"
)

type User struct {
	Quota    quota.Quota
	Email    string
	Password string
	APIKey   string
	Roles    []RoleInstance
	Groups   []string
	// FromToken denotes whether the user was generated from team token.
	// In other words, it does not exist in the storage.
	FromToken bool
	Disabled  bool

	APIKeyLastAccess   time.Time
	APIKeyUsageCounter int64
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
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidKey         = errors.New("invalid key")
	ErrKeyDisabled        = errors.New("key management is disabled")
	ErrEmailFromTeamToken = errors.New("email from team token")
)

func (e *ErrTeamStillUsed) Error() string {
	if len(e.Apps) > 0 {
		return fmt.Sprintf("Apps: %s", strings.Join(e.Apps, ", "))
	}
	return fmt.Sprintf("Service instances: %s", strings.Join(e.ServiceInstances, ", "))
}
