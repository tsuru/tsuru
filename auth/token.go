// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"crypto"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"labix.org/v2/mgo/bson"
	"strings"
	"time"
)

const keySize = 32

var ErrInvalidToken = errors.New("Invalid token")

type Token struct {
	Token     string        `json:"token"`
	Creation  time.Time     `json:"creation"`
	Expires   time.Duration `json:"expires"`
	UserEmail string        `json:"email"`
	AppName   string        `json:"app"`
}

func (t *Token) User() (*User, error) {
	return GetUserByEmail(t.UserEmail)
}

func (t *Token) IsAppToken() bool {
	return t.AppName != ""
}

type passwordToken struct {
	Token     string `bson:"_id"`
	UserEmail string
	Creation  time.Time
	Used      bool
}

// parseToken extracs token from a header:
// 'type token' or 'token'
func parseToken(header string) (string, error) {
	s := strings.Split(header, " ")
	if len(s) == 2 {
		return s[1], nil
	}
	return "", ErrInvalidToken
}

func GetToken(header string) (*Token, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var t Token
	token, err := parseToken(header)
	if err != nil {
		return nil, err
	}
	err = conn.Tokens().Find(bson.M{"token": token}).One(&t)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if t.Creation.Add(t.Expires).Sub(time.Now()) < 1 {
		return nil, ErrInvalidToken
	}
	return &t, nil
}

func DeleteToken(token string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Tokens().Remove(bson.M{"token": token})
}

func CreateApplicationToken(appName string) (*Token, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	t := Token{
		Token:    token(appName, crypto.SHA1),
		Creation: time.Now(),
		Expires:  365 * 24 * time.Hour,
		AppName:  appName,
	}
	err = conn.Tokens().Insert(t)
	if err != nil {
		return nil, err
	}
	return &t, nil
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

func newUserToken(u *User) (*Token, error) {
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

func createPasswordToken(u *User) (*passwordToken, error) {
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
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.PasswordTokens().Insert(t)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (t *passwordToken) user() (*User, error) {
	return GetUserByEmail(t.UserEmail)
}

func getPasswordToken(token string) (*passwordToken, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var t passwordToken
	err = conn.PasswordTokens().Find(bson.M{"_id": token, "used": false}).One(&t)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if t.Creation.Add(24*time.Hour).Sub(time.Now()) < time.Minute {
		return nil, ErrInvalidToken
	}
	return &t, nil
}

func removeOldTokens(userEmail string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var limit int
	if limit, err = config.GetInt("auth:max-simultaneous-sessions"); err != nil {
		return err
	}
	count, err := conn.Tokens().Find(bson.M{"useremail": userEmail}).Count()
	if err != nil {
		return err
	}
	diff := count - limit
	if diff < 1 {
		return nil
	}
	var tokens []map[string]interface{}
	err = conn.Tokens().Find(bson.M{"useremail": userEmail}).Select(bson.M{"_id": 1}).Limit(diff).All(&tokens)
	if err != nil {
		return nil
	}
	ids := make([]interface{}, 0, len(tokens))
	for _, token := range tokens {
		ids = append(ids, token["_id"])
	}
	_, err = conn.Tokens().RemoveAll(bson.M{"_id": bson.M{"$in": ids}})
	return err
}
