// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"context"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/permission"
	authTypes "github.com/tsuru/tsuru/types/auth"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/oauth2"
)

var _ authTypes.Token = &tokenWrapper{}

type tokenWrapper struct {
	oauth2.Token
	UserEmail string `json:"email"`
}

func (t *tokenWrapper) GetValue() string {
	return t.AccessToken
}

func (t *tokenWrapper) User(ctx context.Context) (*authTypes.User, error) {
	return auth.ConvertOldUser(auth.GetUserByEmail(ctx, t.UserEmail))
}

func (t *tokenWrapper) GetUserName() string {
	return t.UserEmail
}

func (t *tokenWrapper) Engine() string {
	return "oauth"
}

func (t *tokenWrapper) Permissions(ctx context.Context) ([]permission.Permission, error) {
	return auth.BaseTokenPermission(ctx, t)
}

func getToken(ctx context.Context, header string) (*tokenWrapper, error) {
	var t tokenWrapper
	token, err := auth.ParseToken(header)
	if err != nil {
		return nil, err
	}

	collection, err := storagev2.OAuth2TokensCollection()
	if err != nil {
		return nil, err
	}

	err = collection.FindOne(ctx, mongoBSON.M{"token.accesstoken": token}).Decode(&t)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, auth.ErrInvalidToken
		}
		return nil, err
	}
	return &t, nil
}

func deleteToken(ctx context.Context, token string) error {
	collection, err := storagev2.OAuth2TokensCollection()
	if err != nil {
		return err
	}
	_, err = collection.DeleteOne(ctx, mongoBSON.M{"token.accesstoken": token})
	if err != nil {
		return err
	}

	return nil
}

func deleteAllTokens(ctx context.Context, email string) error {
	collection, err := storagev2.OAuth2TokensCollection()
	if err != nil {
		return err
	}
	_, err = collection.DeleteMany(ctx, mongoBSON.M{"useremail": email})
	return err
}

func (t *tokenWrapper) save(ctx context.Context) error {
	collection, err := storagev2.OAuth2TokensCollection()
	if err != nil {
		return err
	}
	_, err = collection.InsertOne(ctx, t)
	return err
}
