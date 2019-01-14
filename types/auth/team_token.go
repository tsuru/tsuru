// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"errors"
	"time"
)

type TeamTokenCreateArgs struct {
	TokenID     string `json:"token_id" form:"token_id"`
	Description string `json:"description" form:"description"`
	ExpiresIn   int    `json:"expires_in" form:"expires_in"`
	Team        string `json:"team" form:"team"`
}

type TeamTokenUpdateArgs struct {
	TokenID     string `json:"token_id" form:"token_id"`
	Regenerate  bool   `json:"regenerate" form:"regenerate"`
	Description string `json:"description" form:"description"`
	ExpiresIn   int    `json:"expires_in" form:"expires_in"`
}

type TeamToken struct {
	Token        string         `json:"token"`
	TokenID      string         `json:"token_id"`
	Description  string         `json:"description"`
	CreatedAt    time.Time      `json:"created_at"`
	ExpiresAt    time.Time      `json:"expires_at"`
	LastAccess   time.Time      `json:"last_access"`
	CreatorEmail string         `json:"creator_email"`
	Team         string         `json:"team"`
	Roles        []RoleInstance `json:"roles,omitempty"`
}

type TeamTokenStorage interface {
	Insert(TeamToken) error
	FindByTokenID(tokenID string) (*TeamToken, error)
	FindByToken(token string) (*TeamToken, error)
	FindByTeams(teams []string) ([]TeamToken, error)
	UpdateLastAccess(token string) error
	Update(TeamToken) error
	Delete(tokenID string) error
}

type TeamTokenService interface {
	Create(args TeamTokenCreateArgs, token Token) (TeamToken, error)
	Info(tokenID string, token Token) (TeamToken, error)
	Update(args TeamTokenUpdateArgs, token Token) (TeamToken, error)
	Delete(tokenID string) error
	Authenticate(header string) (Token, error)
	FindByTokenID(tokenID string) (TeamToken, error)
	FindByUserToken(t Token) ([]TeamToken, error)
	AddRole(tokenID string, roleName, contextValue string) error
	RemoveRole(tokenID string, roleName, contextValue string) error
}

var (
	ErrTeamTokenAlreadyExists = errors.New("team token already exists")
	ErrTeamTokenNotFound      = errors.New("team token not found")
	ErrTeamTokenExpired       = errors.New("team token expired")
)
