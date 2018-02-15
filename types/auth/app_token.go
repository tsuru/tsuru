// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"crypto"
	"crypto/rand"
	"errors"
	"fmt"
	"time"
)

type AppToken struct {
	Token        string     `json:"token"`
	CreatedAt    time.Time  `json:"created_at"`
	ExpiresAt    *time.Time `json:"expires_at"`
	LastAccess   *time.Time `json:"last_access"`
	CreatorEmail string     `json:"creator_email"`
	AppName      string     `json:"app"`
	Roles        []string   `json:"roles,omitempty"`
}

type AppTokenService interface {
	Insert(AppToken) error
	FindByToken(string) (*AppToken, error)
	FindByAppName(string) ([]AppToken, error)
	Authenticate(string) (*AppToken, error)
	AddRoles(AppToken, ...string) error
	RemoveRoles(AppToken, ...string) error
	Delete(AppToken) error
}

var (
	ErrAppTokenAlreadyExists = errors.New("app token already exists")
	ErrAppTokenNotFound      = errors.New("app token not found")
	ErrAppTokenExpired       = errors.New("app token expired")
)

func NewAppToken(appName, creatorEmail string) AppToken {
	// TODO: config expiration
	now := time.Now()
	expiresAt := now.Add(365 * 24 * time.Hour)
	return AppToken{
		Token:        generateToken(appName, crypto.SHA1),
		AppName:      appName,
		CreatorEmail: creatorEmail,
		CreatedAt:    now,
		ExpiresAt:    &expiresAt,
	}
}

// TODO: extract token function from auth/native/token.go
const keySize = 32

func generateToken(data string, hash crypto.Hash) string {
	var tokenKey [keySize]byte
	n, err := rand.Read(tokenKey[:])
	for n < keySize || err != nil {
		n, err = rand.Read(tokenKey[:])
	}
	h := hash.New()
	h.Write([]byte(data))
	h.Write(tokenKey[:])
	h.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (t *AppToken) AddRole(roleName string) {
	for _, r := range t.Roles {
		if r == roleName {
			return
		}
	}
	t.Roles = append(t.Roles, roleName)
}
