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

type TeamToken struct {
	Token        string     `json:"token"`
	CreatedAt    time.Time  `json:"created_at"`
	ExpiresAt    *time.Time `json:"expires_at"`
	LastAccess   *time.Time `json:"last_access"`
	CreatorEmail string     `json:"creator_email"`
	AppName      string     `json:"app"`
	Teams        []string   `json:"teams"`
	Roles        []string   `json:"roles,omitempty"`
}

type TeamTokenService interface {
	Insert(TeamToken) error
	FindByToken(string) (*TeamToken, error)
	FindByAppName(string) ([]TeamToken, error)
	Authenticate(string) (*TeamToken, error)
	AddTeams(TeamToken, ...string) error
	RemoveTeams(TeamToken, ...string) error
	AddRoles(TeamToken, ...string) error
	RemoveRoles(TeamToken, ...string) error
	Delete(TeamToken) error
}

var (
	ErrTeamTokenAlreadyExists = errors.New("team token already exists")
	ErrTeamTokenNotFound      = errors.New("team token not found")
	ErrTeamTokenExpired       = errors.New("team token expired")
)

func NewTeamToken(appName, creatorEmail string) TeamToken {
	// TODO: config expiration
	now := time.Now()
	expiresAt := now.Add(365 * 24 * time.Hour)
	return TeamToken{
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

func (t *TeamToken) AddRole(roleName string) {
	for _, r := range t.Roles {
		if r == roleName {
			return
		}
	}
	t.Roles = append(t.Roles, roleName)
}
