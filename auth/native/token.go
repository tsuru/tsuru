// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"code.google.com/p/go.crypto/bcrypt"
	"crypto"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/validation"
	"labix.org/v2/mgo/bson"
	"strings"
	"time"
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
	AppName   string        `json:"app"`
}

func (t *Token) GetValue() string {
	return t.Token
}

func (t *Token) User() (*auth.User, error) {
	return auth.GetUserByEmail(t.UserEmail)
}

func (t *Token) IsAppToken() bool {
	return t.AppName != ""
}

func (t *Token) GetUserName() string {
	return t.UserEmail
}

func (t *Token) GetAppName() string {
	return t.AppName
}

func loadConfig() error {
	if cost == 0 && tokenExpire == 0 {
		var err error
		if days, err := config.GetInt("auth:token-expire-days"); err == nil {
			tokenExpire = time.Duration(int64(days) * 24 * int64(time.Hour))
		} else {
			tokenExpire = defaultExpiration
		}
		if cost, err = config.GetInt("auth:hash-cost"); err != nil {
			cost = bcrypt.DefaultCost
		}
		if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
			return fmt.Errorf("Invalid value for setting %q: it must be between %d and %d.", "auth:hash-cost", bcrypt.MinCost, bcrypt.MaxCost)
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

func checkPassword(passwordHash string, password string) error {
	if !validation.ValidateLength(password, passwordMinLen, passwordMaxLen) {
		return &tsuruErrors.ValidationError{Message: passwordError}
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) == nil {
		return nil
	}
	return auth.AuthenticationFailure{Message: "Authentication failed, wrong password."}
}

func createToken(u *auth.User, password string) (*Token, error) {
	if u.Email == "" {
		return nil, errors.New("User does not have an email")
	}
	if err := checkPassword(u.Password, password); err != nil {
		return nil, err
	}
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	token, err := newUserToken(u)
	if err != nil {
		return nil, err
	}
	err = conn.Tokens().Insert(token)
	go removeOldTokens(u.Email)
	return token, err
}

// parseToken extracts token from a header:
// 'type token' or 'token'
func parseToken(header string) (string, error) {
	s := strings.Split(header, " ")
	var value string
	if len(s) < 3 {
		value = s[len(s)-1]
	}
	if value != "" {
		return value, nil
	}
	return value, auth.ErrInvalidToken
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
		return nil, auth.ErrInvalidToken
	}
	if t.Creation.Add(t.Expires).Sub(time.Now()) < 1 {
		return nil, auth.ErrInvalidToken
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

func DeleteAllTokens(email string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Tokens().RemoveAll(bson.M{"useremail": email})
	return err
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
