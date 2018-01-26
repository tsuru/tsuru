// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"errors"
	"time"
)

type AppToken struct {
	Token        string        `json:"token"`
	Creation     time.Time     `json:"creation"`
	Expires      time.Duration `json:"expires"`
	LastAccess   time.Time     `json:"last_access"`
	CreatorEmail string        `json:"email"`
	AppName      string        `json:"app"`
}

type AppTokenService interface {
	Insert(AppToken) error
	FindByToken(string) (*AppToken, error)
	FindByAppName(string) ([]AppToken, error)
	Delete(AppToken) error
}

var (
	ErrAppTokenAlreadyExists = errors.New("app token already exists")
	ErrAppTokenNotFound      = errors.New("app token not found")
)
