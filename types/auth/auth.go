// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

type SchemeInfo struct {
	Name    string     `json:"name"`
	Default bool       `json:"default,omitempty"`
	Data    SchemeData `json:"data"`
}

type SchemeData struct {
	// OIDC fields
	ClientID string   `json:"clientID,omitempty"`
	Scopes   []string `json:"scopes,omitempty"`
	AuthURL  string   `json:"authURL,omitempty"`
	TokenURL string   `json:"tokenURL,omitempty"`
	Port     string   `json:"port,omitempty"`

	// OAuth fields
	AuthorizeURL string `json:"authorizeUrl,omitempty"`
}
