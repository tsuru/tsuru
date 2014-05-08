// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"encoding/json"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"io/ioutil"
	"net/http"
	"net/url"
)

type Token struct {
	Token string
}

type client struct {
	url string
}

func (c *client) getToken(code string) (string, error) {
	clientId, err := config.GetString("auth:oauth:client-id")
	if err != nil {
		return "", err
	}
	clientSecret, err := config.GetString("auth:oauth:client-secret")
	if err != nil {
		return "", err
	}
	v := url.Values{}
	v.Add("code", code)
	v.Add("client_id", clientId)
	v.Add("client_secret", clientSecret)
	resp, err := http.PostForm(c.url, v)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	data := map[string]string{}
	json.Unmarshal(body, &data)
	return data["access_token"], nil

}

func newToken(code string) (*Token, error) {
	t := Token{}
	t.Token = ""
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Tokens().Insert(&t)
	return &t, err
}
