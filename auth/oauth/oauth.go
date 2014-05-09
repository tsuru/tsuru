// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"code.google.com/p/goauth2/oauth"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"net/http"
)

var (
	ErrMissingCodeError = &tsuruErrors.ValidationError{Message: "You must provide code to login"}
)

type OAuthParser interface {
	Parse(oauthToken *oauth.Token, infoResponse *http.Response) (auth.Token, error)
}

type OAuthScheme struct {
	Config  *oauth.Config
	InfoUrl string
	Parser  OAuthParser
}

func init() {
	auth.RegisterScheme("oauth", &OAuthScheme{})
}

func (s *OAuthScheme) loadConfig() error {
	if s.Config != nil {
		return nil
	}
	if s.Parser == nil {
		s.Parser = s
	}
	clientId, err := config.GetString("auth:oauth:client-id")
	if err != nil {
		return err
	}
	clientSecret, err := config.GetString("auth:oauth:client-secret")
	if err != nil {
		return err
	}
	scope, err := config.GetString("auth:oauth:scope")
	if err != nil {
		return err
	}
	authURL, err := config.GetString("auth:oauth:auth-url")
	if err != nil {
		return err
	}
	tokenURL, err := config.GetString("auth:oauth:token-url")
	if err != nil {
		return err
	}
	infoURL, err := config.GetString("auth:oauth:info-url")
	if err != nil {
		return err
	}
	s.InfoUrl = infoURL
	s.Config = &oauth.Config{
		ClientId:     clientId,
		ClientSecret: clientSecret,
		RedirectURL:  "{{redirect_url}}",
		Scope:        scope,
		AuthURL:      authURL,
		TokenURL:     tokenURL,
	}
	return nil
}

func (s *OAuthScheme) Login(params map[string]string) (auth.Token, error) {
	if err := s.loadConfig(); err != nil {
		return nil, err
	}
	code, ok := params["code"]
	if !ok {
		return nil, ErrMissingCodeError
	}
	transport := &oauth.Transport{Config: s.Config}
	token, err := transport.Exchange(code)
	if err != nil {
		return nil, err
	}
	transport.Token = token
	client := transport.Client()
	response, err := client.Get(s.InfoUrl)
	if err != nil {
		return nil, err
	}
	authToken, err := s.Parser.Parse(token, response)
	if err != nil {
		return nil, err
	}
	return authToken, nil
}

func (s *OAuthScheme) AppLogin(appName string) (auth.Token, error) {
	if err := s.loadConfig(); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *OAuthScheme) Logout(token string) error {
	if err := s.loadConfig(); err != nil {
		return err
	}
	return nil
}

func (s *OAuthScheme) Auth(token string) (auth.Token, error) {
	if err := s.loadConfig(); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *OAuthScheme) Name() string {
	return "oauth"
}

func (s *OAuthScheme) Info() (auth.SchemeInfo, error) {
	if err := s.loadConfig(); err != nil {
		return nil, err
	}
	return auth.SchemeInfo{"authorizeUrl": s.Config.AuthCodeURL("")}, nil
}

func (s *OAuthScheme) Parse(oauthToken *oauth.Token, infoResponse *http.Response) (auth.Token, error) {
	t := &Token{Token: *oauthToken, UserEmail: "x@x.com"}
	t.save()
	// TODO: save user
	return t, nil
}
