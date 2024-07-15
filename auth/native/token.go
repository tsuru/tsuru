// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"context"
	"crypto"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db/storagev2"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/permission"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/validation"
	mongoBSON "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

const (
	keySize           = 32
	defaultExpiration = 7 * 24 * time.Hour
	passwordError     = "Password length should be least 6 characters and at most 50 characters."
	passwordMinLen    = 6
	passwordMaxLen    = 50
)

var (
	tokenExpire time.Duration
	cost        int
)

type Token struct {
	Token     string        `json:"token"`
	Creation  time.Time     `json:"creation"`
	Expires   time.Duration `json:"expires"`
	UserEmail string        `json:"email"`
}

func (t *Token) GetValue() string {
	return t.Token
}

func (t *Token) User(ctx context.Context) (*authTypes.User, error) {
	return auth.ConvertOldUser(auth.GetUserByEmail(ctx, t.UserEmail))
}

func (t *Token) GetUserName() string {
	return t.UserEmail
}

func (t *Token) Engine() string {
	return "native"
}
func (t *Token) Permissions(ctx context.Context) ([]permission.Permission, error) {
	return auth.BaseTokenPermission(ctx, t)
}

func loadConfig() error {
	if cost == 0 && tokenExpire == 0 {
		var err error
		var days int
		if days, err = config.GetInt("auth:token-expire-days"); err == nil {
			tokenExpire = time.Duration(int64(days) * 24 * int64(time.Hour))
		} else {
			tokenExpire = defaultExpiration
		}
		if cost, err = config.GetInt("auth:hash-cost"); err != nil {
			cost = bcrypt.DefaultCost
		}
		if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
			return errors.Errorf("Invalid value for setting %q: it must be between %d and %d.", "auth:hash-cost", bcrypt.MinCost, bcrypt.MaxCost)
		}
	}
	return nil
}

func hashPassword(u *auth.User) error {
	loadConfig()
	passwd, err := bcrypt.GenerateFromPassword([]byte(u.Password), cost)
	if err != nil {
		return err
	}
	u.Password = string(passwd)
	return nil
}

func token(data string, hash crypto.Hash) string {
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

func newUserToken(u *auth.User) (*Token, error) {
	if u == nil {
		return nil, errors.New("User is nil")
	}
	if u.Email == "" {
		return nil, errors.New("Impossible to generate tokens for users without email")
	}
	if err := loadConfig(); err != nil {
		return nil, err
	}
	t := Token{}
	t.Creation = time.Now()
	t.Expires = tokenExpire
	t.Token = token(u.Email, crypto.SHA1)
	t.UserEmail = u.Email
	return &t, nil
}

func removeOldTokens(ctx context.Context, userEmail string) error {
	collection, err := storagev2.TokensCollection()
	if err != nil {
		return err
	}
	var limit int
	if limit, err = config.GetInt("auth:max-simultaneous-sessions"); err != nil {
		return err
	}
	count, err := collection.CountDocuments(ctx, mongoBSON.M{"useremail": userEmail})
	if err != nil {
		return err
	}
	diff := count - int64(limit)
	if diff < 1 {
		return nil
	}
	var tokens []map[string]interface{}

	opts := options.Find().SetSort(bson.M{"creation": 1}).SetLimit(int64(diff)).SetProjection(bson.M{"_id": 1})
	cursor, err := collection.Find(ctx, mongoBSON.M{"useremail": userEmail}, opts)
	if err != nil {
		return nil
	}
	err = cursor.All(ctx, &tokens)
	if err != nil {
		return nil
	}
	ids := make([]interface{}, 0, len(tokens))
	for _, token := range tokens {
		ids = append(ids, token["_id"])
	}
	_, err = collection.DeleteMany(ctx, mongoBSON.M{"_id": mongoBSON.M{"$in": ids}})
	return err
}

func checkPassword(passwordHash string, password string) error {
	if !validation.ValidateLength(password, passwordMinLen, passwordMaxLen) {
		return &tsuruErrors.ValidationError{Message: passwordError}
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) == nil {
		return nil
	}
	return auth.AuthenticationFailure{Message: "Authentication failed, wrong password."}
}

func createToken(ctx context.Context, u *auth.User, password string) (*Token, error) {
	if u.Email == "" {
		return nil, errors.New("User does not have an email")
	}
	if err := checkPassword(u.Password, password); err != nil {
		return nil, err
	}
	collection, err := storagev2.TokensCollection()
	if err != nil {
		return nil, err
	}
	token, err := newUserToken(u)
	if err != nil {
		return nil, err
	}
	_, err = collection.InsertOne(ctx, token)
	go removeOldTokens(ctx, u.Email)
	return token, err
}

func getToken(ctx context.Context, header string) (*Token, error) {
	collection, err := storagev2.TokensCollection()
	if err != nil {
		return nil, err
	}
	var t Token
	token, err := auth.ParseToken(header)
	if err != nil {
		return nil, err
	}
	err = collection.FindOne(ctx, mongoBSON.M{"token": token}).Decode(&t)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, auth.ErrInvalidToken
		}
		return nil, err
	}
	if t.Expires > 0 && time.Until(t.Creation.Add(t.Expires)) < 1 {
		return nil, auth.ErrInvalidToken
	}
	return &t, nil
}

func deleteToken(ctx context.Context, token string) error {
	collection, err := storagev2.TokensCollection()
	if err != nil {
		return err
	}
	_, err = collection.DeleteOne(ctx, mongoBSON.M{"token": token})
	if err != nil {
		return err
	}
	return nil
}

func deleteAllTokens(ctx context.Context, email string) error {
	collection, err := storagev2.TokensCollection()
	if err != nil {
		return err
	}
	_, err = collection.DeleteMany(ctx, mongoBSON.M{"useremail": email})
	return err
}
