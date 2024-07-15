// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"context"
	"time"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	check "gopkg.in/check.v1"
)

func (s *S) TestCreatePasswordToken(c *check.C) {
	passwordTokensCollection, err := storagev2.PasswordTokensCollection()
	c.Assert(err, check.IsNil)

	u := auth.User{Email: "pure@alanis.com"}
	t, err := createPasswordToken(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	c.Assert(t.UserEmail, check.Equals, u.Email)
	c.Assert(t.Used, check.Equals, false)
	var dbToken passwordToken
	err = passwordTokensCollection.FindOne(context.TODO(), mongoBSON.M{"_id": t.Token}).Decode(&dbToken)
	c.Assert(err, check.IsNil)
	c.Assert(dbToken.Token, check.Equals, t.Token)
	c.Assert(dbToken.UserEmail, check.Equals, t.UserEmail)
	c.Assert(dbToken.Used, check.Equals, t.Used)
}

func (s *S) TestCreatePasswordTokenErrors(c *check.C) {
	var tests = []struct {
		input *auth.User
		want  string
	}{
		{nil, "User is nil"},
		{&auth.User{}, "User email is empty"},
	}
	for _, t := range tests {
		token, err := createPasswordToken(context.TODO(), t.input)
		c.Check(token, check.IsNil)
		c.Check(err, check.NotNil)
		c.Check(err.Error(), check.Equals, t.want)
	}
}

func (s *S) TestPasswordTokenUser(c *check.C) {
	u := auth.User{Email: "need@who.com", Password: "123456"}
	err := u.Create(context.TODO())
	c.Assert(err, check.IsNil)
	defer u.Delete(context.TODO())
	t, err := createPasswordToken(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	u2, err := t.user(context.TODO())
	c.Assert(err, check.IsNil)
	c.Assert(*u2, check.DeepEquals, u)
}

func (s *S) TestGetPasswordToken(c *check.C) {
	u := auth.User{Email: "porcelain@opeth.com"}
	t, err := createPasswordToken(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	t2, err := getPasswordToken(context.TODO(), t.Token)
	t2.Creation = t.Creation
	c.Assert(err, check.IsNil)
	c.Assert(t2, check.DeepEquals, t)
}

func (s *S) TestGetPasswordTokenUnknown(c *check.C) {
	t, err := getPasswordToken(context.TODO(), "what??")
	c.Assert(t, check.IsNil)
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestGetPasswordUsedToken(c *check.C) {
	passwordTokensCollection, err := storagev2.PasswordTokensCollection()
	c.Assert(err, check.IsNil)

	u := auth.User{Email: "porcelain@opeth.com"}
	t, err := createPasswordToken(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	t.Used = true
	_, err = passwordTokensCollection.ReplaceOne(context.TODO(), mongoBSON.M{"_id": t.Token}, t)
	c.Assert(err, check.IsNil)
	t2, err := getPasswordToken(context.TODO(), t.Token)
	c.Assert(t2, check.IsNil)
	c.Assert(err, check.Equals, auth.ErrInvalidToken)
}

func (s *S) TestPasswordTokensAreValidFor24Hours(c *check.C) {
	passwordTokensCollection, err := storagev2.PasswordTokensCollection()
	c.Assert(err, check.IsNil)

	u := auth.User{Email: "porcelain@opeth.com"}
	t, err := createPasswordToken(context.TODO(), &u)
	c.Assert(err, check.IsNil)
	t.Creation = time.Now().Add(-24 * time.Hour)
	_, err = passwordTokensCollection.ReplaceOne(context.TODO(), mongoBSON.M{"_id": t.Token}, t)
	c.Assert(err, check.IsNil)
	t2, err := getPasswordToken(context.TODO(), t.Token)
	c.Assert(t2, check.IsNil)
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, "Invalid token")
}
