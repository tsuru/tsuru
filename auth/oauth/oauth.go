// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	goauth2 "code.google.com/p/goauth2/oauth"
	"encoding/json"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"net/http"
)

var (
	ErrMissingCodeError       = &tsuruErrors.ValidationError{Message: "You must provide code to login"}
	ErrMissingCodeRedirectUrl = &tsuruErrors.ValidationError{Message: "You must provide the used redirect url to login"}
	ErrEmptyAccessToken       = &tsuruErrors.NotAuthorizedError{Message: "Couldn't convert code to access token."}
	ErrEmptyUserEmail         = &tsuruErrors.NotAuthorizedError{Message: "Couldn't parse user email."}
)

type OAuthParser interface {
	Parse(infoResponse *http.Response) (string, error)
}

type OAuthScheme struct {
	Config       *goauth2.Config
	InfoUrl      string
	CallbackPort string
	Parser       OAuthParser
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
	callbackPort, err := config.GetString("auth:oauth:callback-port")
	if err != nil {
		log.Debugf("auth:oauth:callback-port not found using random port.")
		callbackPort = ""
	}
	s.InfoUrl = infoURL
	s.CallbackPort = callbackPort
	s.Config = &goauth2.Config{
		ClientId:     clientId,
		ClientSecret: clientSecret,
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
	redirectUrl, ok := params["redirectUrl"]
	if !ok {
		return nil, ErrMissingCodeRedirectUrl
	}
	s.Config.RedirectURL = redirectUrl
	transport := &goauth2.Transport{Config: s.Config}
	oauthToken, err := transport.Exchange(code)
	if err != nil {
		return nil, err
	}
	if oauthToken.AccessToken == "" {
		return nil, ErrEmptyAccessToken
	}
	transport.Token = oauthToken
	client := transport.Client()
	response, err := client.Get(s.InfoUrl)
	if err != nil {
		return nil, err
	}
	email, err := s.Parser.Parse(response)
	if email == "" {
		return nil, ErrEmptyUserEmail
	}
	_, err = auth.GetUserByEmail(email)
	if err != nil {
		if err != auth.ErrUserNotFound {
			return nil, err
		}
		user := auth.User{Email: email}
		err = user.Create()
		if err != nil {
			return nil, err
		}
	}
	authToken := &Token{Token: *oauthToken, UserEmail: email}
	err = authToken.save()
	if err != nil {
		return nil, err
	}
	return authToken, nil
}

func (s *OAuthScheme) AppLogin(appName string) (auth.Token, error) {
	nativeScheme := native.NativeScheme{}
	return nativeScheme.AppLogin(appName)
}

func (s *OAuthScheme) Logout(token string) error {
	return deleteToken(token)
}

func (s *OAuthScheme) Auth(header string) (auth.Token, error) {
	if err := s.loadConfig(); err != nil {
		return nil, err
	}
	token, err := getToken(header)
	if err != nil {
		nativeScheme := native.NativeScheme{}
		token, nativeErr := nativeScheme.Auth(header)
		if nativeErr == nil && token.IsAppToken() {
			return token, nil
		}
		return nil, err
	}
	transport := goauth2.Transport{Config: s.Config}
	transport.Token = &token.Token
	client := transport.Client()
	_, err = client.Get(s.InfoUrl)
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (s *OAuthScheme) Name() string {
	return "oauth"
}

func (s *OAuthScheme) Info() (auth.SchemeInfo, error) {
	if err := s.loadConfig(); err != nil {
		return nil, err
	}
	config := new(goauth2.Config)
	*config = *s.Config
	config.RedirectURL = "%s"
	return auth.SchemeInfo{"authorizeUrl": config.AuthCodeURL(""), "port": s.CallbackPort}, nil
}

func (s *OAuthScheme) Parse(infoResponse *http.Response) (string, error) {
	user := struct {
		Email string `json:"email"`
	}{}
	err := json.NewDecoder(infoResponse.Body).Decode(&user)
	if err != nil {
		return user.Email, err
	}
	return user.Email, nil
}
