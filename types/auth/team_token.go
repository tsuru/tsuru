// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
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
	Insert(context.Context, TeamToken) error
	FindByTokenID(ctx context.Context, tokenID string) (*TeamToken, error)
	FindByToken(ctx context.Context, token string) (*TeamToken, error)
	FindByTeams(ctx context.Context, teams []string) ([]TeamToken, error)
	UpdateLastAccess(ctx context.Context, token string) error
	Update(context.Context, TeamToken) error
	Delete(ctx context.Context, tokenID string) error
}

type TeamTokenService interface {
	Create(ctx context.Context, args TeamTokenCreateArgs, token Token) (TeamToken, error)
	Info(ctx context.Context, tokenID string, token Token) (TeamToken, error)
	Update(ctx context.Context, args TeamTokenUpdateArgs, token Token) (TeamToken, error)
	Delete(ctx context.Context, tokenID string) error
	Authenticate(ctx context.Context, header string) (Token, error)
	FindByTokenID(ctx context.Context, tokenID string) (TeamToken, error)
	FindByUserToken(ctx context.Context, t Token) ([]TeamToken, error)
	AddRole(ctx context.Context, tokenID string, roleName, contextValue string) error
	RemoveRole(ctx context.Context, tokenID string, roleName, contextValue string) error
}

var (
	ErrTeamTokenAlreadyExists = errors.New("team token already exists")
	ErrTeamTokenNotFound      = errors.New("team token not found")
	ErrTeamTokenExpired       = errors.New("team token expired")
)
