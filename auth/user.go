// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"bytes"
	"code.google.com/p/go.crypto/bcrypt"
	stderrors "errors"
	"fmt"
	"github.com/tsuru/config"
	"github.com/tsuru/go-gandalfclient"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/quota"
	"github.com/tsuru/tsuru/repository"
	"github.com/tsuru/tsuru/validation"
	"labix.org/v2/mgo/bson"
	"math/rand"
	"net"
	"net/smtp"
	"strings"
	"time"
)

const (
	defaultExpiration = 7 * 24 * time.Hour
	emailError        = "Invalid email."
	passwordError     = "Password length should be least 6 characters and at most 50 characters."
	passwordMinLen    = 6
	passwordMaxLen    = 50
)

var ErrUserNotFound = stderrors.New("User not found")

var tokenExpire time.Duration
var cost int

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

type Key struct {
	Name    string
	Content string
}

type User struct {
	Email    string
	Password string
	Keys     []Key
	quota.Quota
}

func GetUserByEmail(email string) (*User, error) {
	if !validation.ValidateEmail(email) {
		return nil, &errors.ValidationError{Message: emailError}
	}
	var u User
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Users().Find(bson.M{"email": email}).One(&u)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return &u, nil
}

func (u *User) Create() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	u.HashPassword()
	return conn.Users().Insert(u)
}

func (u *User) Update() error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Users().Update(bson.M{"email": u.Email}, u)
}

func (u *User) HashPassword() {
	loadConfig()
	if passwd, err := bcrypt.GenerateFromPassword([]byte(u.Password), cost); err == nil {
		u.Password = string(passwd)
	}
}

func (u *User) CheckPassword(password string) error {
	if !validation.ValidateLength(password, passwordMinLen, passwordMaxLen) {
		return &errors.ValidationError{Message: passwordError}
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)) == nil {
		return nil
	}
	return AuthenticationFailure{}
}

// Teams returns a slice containing all teams that the user is member of.
func (u *User) Teams() ([]Team, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var teams []Team
	err = conn.Teams().Find(bson.M{"users": u.Email}).All(&teams)
	if err != nil {
		return nil, err
	}
	return teams, nil
}

func (u *User) FindKey(key Key) (Key, int) {
	for i, k := range u.Keys {
		if k.Content == key.Content {
			return k, i
		}
	}
	return Key{}, -1
}

func (u *User) HasKey(key Key) bool {
	_, index := u.FindKey(key)
	return index > -1
}

func (u *User) AddKey(key Key) error {
	u.Keys = append(u.Keys, key)
	return nil
}

func (u *User) RemoveKey(key Key) error {
	_, index := u.FindKey(key)
	if index < 0 {
		return stderrors.New("Key not found")
	}
	copy(u.Keys[index:], u.Keys[index+1:])
	u.Keys = u.Keys[:len(u.Keys)-1]
	return nil
}

func (u *User) IsAdmin() bool {
	adminTeamName, err := config.GetString("admin-team")
	if err != nil {
		return false
	}
	teams, err := u.Teams()
	if err != nil {
		return false
	}
	for _, t := range teams {
		if t.Name == adminTeamName {
			return true
		}
	}
	return false
}

func (u *User) AllowedApps() ([]string, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var alwdApps []map[string]string
	teams, err := u.Teams()
	if err != nil {
		return nil, err
	}
	teamNames := GetTeamsNames(teams)
	q := bson.M{"teams": bson.M{"$in": teamNames}}
	if err := conn.Apps().Find(q).Select(bson.M{"name": 1}).All(&alwdApps); err != nil {
		return nil, err
	}
	appNames := make([]string, len(alwdApps))
	for i, v := range alwdApps {
		appNames[i] = v["name"]
	}
	return appNames, nil
}

// StartPasswordReset starts the password reset process, creating a new token
// and mailing it to the user.
//
// The token should then be used to finish the process, through the
// ResetPassword function.
func (u *User) StartPasswordReset() error {
	t, err := createPasswordToken(u)
	if err != nil {
		return err
	}
	go u.sendResetPassword(t)
	return nil
}

func (u *User) sendResetPassword(t *passwordToken) {
	var body bytes.Buffer
	err := resetEmailData.Execute(&body, t)
	if err != nil {
		log.Errorf("Failed to send password token to user %q: %s", u.Email, err)
		return
	}
	err = sendEmail(u.Email, body.Bytes())
	if err != nil {
		log.Errorf("Failed to send password token for user %q: %s", u.Email, err)
	}
}

// ResetPassword actually resets the password of the user. It needs the token
// string. The new password will be a random string, that will be then sent to
// the user email.
func (u *User) ResetPassword(token string) error {
	if token == "" {
		return ErrInvalidToken
	}
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	t, err := getPasswordToken(token)
	if err != nil {
		return err
	}
	if t.UserEmail != u.Email {
		return ErrInvalidToken
	}
	password := generatePassword(12)
	u.Password = password
	u.HashPassword()
	go u.sendNewPassword(password)
	t.Used = true
	conn.PasswordTokens().UpdateId(t.Token, t)
	return u.Update()
}

func (u *User) sendNewPassword(password string) {
	m := map[string]string{
		"password": password,
		"email":    u.Email,
	}
	var body bytes.Buffer
	err := passwordResetConfirm.Execute(&body, m)
	if err != nil {
		log.Errorf("Failed to send new password to user %q: %s", u.Email, err)
		return
	}
	err = sendEmail(u.Email, body.Bytes())
	if err != nil {
		log.Errorf("Failed to send new password to user %q: %s", u.Email, err)
	}
}

func (u *User) ListKeys() (map[string]string, error) {
	gURL := repository.ServerURL()
	c := gandalf.Client{Endpoint: gURL}
	return c.ListKeys(u.Email)
}

type AuthenticationFailure struct {
	Message string
}

func (a AuthenticationFailure) Error() string {
	if a.Message != "" {
		return a.Message
	}
	return "Authentication failed, wrong password."
}

func generatePassword(length int) string {
	password := make([]byte, length)
	for i := range password {
		password[i] = passwordChars[rand.Int()%len(passwordChars)]
	}
	return string(password)
}

func sendEmail(email string, data []byte) error {
	addr, err := smtpServer()
	if err != nil {
		return err
	}
	var auth smtp.Auth
	user, err := config.GetString("smtp:user")
	if err != nil {
		return stderrors.New(`Setting "smtp:user" is not defined`)
	}
	password, _ := config.GetString("smtp:password")
	if password != "" {
		host, _, _ := net.SplitHostPort(addr)
		auth = smtp.PlainAuth("", user, password, host)
	}
	return smtp.SendMail(addr, auth, user, []string{email}, data)
}

func smtpServer() (string, error) {
	server, _ := config.GetString("smtp:server")
	if server == "" {
		return "", stderrors.New(`Setting "smtp:server" is not defined`)
	}
	if !strings.Contains(server, ":") {
		server += ":25"
	}
	return server, nil
}
