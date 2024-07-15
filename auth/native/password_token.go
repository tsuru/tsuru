// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"context"
	"crypto"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
)

type passwordToken struct {
	Token     string `bson:"_id"`
	UserEmail string
	Creation  time.Time
	Used      bool
}

func createPasswordToken(ctx context.Context, u *auth.User) (*passwordToken, error) {
	if u == nil {
		return nil, errors.New("User is nil")
	}
	if u.Email == "" {
		return nil, errors.New("User email is empty")
	}
	t := passwordToken{
		Token:     token(u.Email, crypto.SHA256),
		UserEmail: u.Email,
		Creation:  time.Now(),
	}

	collection, err := storagev2.PasswordTokensCollection()
	if err != nil {
		return nil, err
	}
	_, err = collection.InsertOne(ctx, t)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (t *passwordToken) user(ctx context.Context) (*auth.User, error) {
	return auth.GetUserByEmail(ctx, t.UserEmail)
}

func getPasswordToken(ctx context.Context, token string) (*passwordToken, error) {
	collection, err := storagev2.PasswordTokensCollection()
	if err != nil {
		return nil, err
	}
	var t passwordToken
	err = collection.FindOne(ctx, mongoBSON.M{"_id": token, "used": mongoBSON.M{"$ne": true}}).Decode(&t)
	if err != nil {
		return nil, auth.ErrInvalidToken
	}
	if time.Until(t.Creation.Add(24*time.Hour)) < time.Minute {
		return nil, auth.ErrInvalidToken
	}
	return &t, nil
}
