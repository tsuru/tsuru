// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/permission"
	authTypes "github.com/tsuru/tsuru/types/auth"
)

type APIToken struct {
	Token     string `json:"token" bson:"apikey"`
	UserEmail string `json:"email" bson:"email"`
}

func (t *APIToken) GetValue() string {
	return t.Token
}

func (t *APIToken) User() (*authTypes.User, error) {
	return ConvertOldUser(GetUserByEmail(t.UserEmail))
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

func APIAuth(header string) (*APIToken, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var t APIToken
	token, err := ParseToken(header)
	if err != nil {
		return nil, err
	}
	err = conn.Users().Find(bson.M{"apikey": token}).One(&t)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, ErrInvalidToken
		}
		return nil, err
	}

	err = conn.Users().Update(bson.M{
		"apikey": token,
	}, bson.M{
		"$set": bson.M{"apikey_last_access": time.Now().UTC()},
		"$inc": bson.M{"apikey_usage_counter": 1},
	})

	if err != nil {
		return nil, err
	}

	return &t, nil
}
