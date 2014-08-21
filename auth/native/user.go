// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"bytes"
	"errors"
	"math/rand"
	"net"
	"net/smtp"
	"strings"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/log"
)

func sendResetPassword(u *auth.User, t *passwordToken) {
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

func sendNewPassword(u *auth.User, password string) {
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
		return errors.New(`Setting "smtp:user" is not defined`)
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
		return "", errors.New(`Setting "smtp:server" is not defined`)
	}
	if !strings.Contains(server, ":") {
		server += ":25"
	}
	return server, nil
}
