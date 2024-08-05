// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"time"

	"github.com/tsuru/tsuru/db/storagev2"
	"github.com/tsuru/tsuru/permission"
	authTypes "github.com/tsuru/tsuru/types/auth"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type APIToken struct {
	Token     string `json:"token" bson:"apikey"`
	UserEmail string `json:"email" bson:"email"`
}

func (t *APIToken) GetValue() string {
	return t.Token
}

func (t *APIToken) User(ctx context.Context) (*authTypes.User, error) {
	return ConvertOldUser(GetUserByEmail(ctx, t.UserEmail))
}

func (t *APIToken) GetUserName() string {
	return t.UserEmail
}

func (t *APIToken) Engine() string {
	return "apikey"
}

func (t *APIToken) Permissions(ctx context.Context) ([]permission.Permission, error) {
	return BaseTokenPermission(ctx, t)
}

func APIAuth(ctx context.Context, header string) (*APIToken, error) {
	usersCollection, err := storagev2.UsersCollection()
	if err != nil {
		return nil, err
	}
	var t APIToken
	token, err := ParseToken(header)
	if err != nil {
		return nil, err
	}
	err = usersCollection.FindOne(ctx, mongoBSON.M{"apikey": token}).Decode(&t)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrInvalidToken
		}
		return nil, err
	}

	_, err = usersCollection.UpdateOne(ctx, mongoBSON.M{
		"apikey": token,
	}, mongoBSON.M{
		"$set": mongoBSON.M{"apikey_last_access": time.Now().UTC()},
		"$inc": mongoBSON.M{"apikey_usage_counter": 1},
	})

	if err != nil {
		return nil, err
	}

	return &t, nil
}
