// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"encoding/json"
	"net/http"
	"strconv"

	goauth2 "code.google.com/p/goauth2/oauth"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
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
	BaseConfig   goauth2.Config
	InfoUrl      string
	CallbackPort int
	Parser       OAuthParser
}

type DBTokenCache struct {
	scheme *OAuthScheme
}

func (c *DBTokenCache) Token() (*goauth2.Token, error) {
	return nil, nil
}

func (c *DBTokenCache) PutToken(t *goauth2.Token) error {
	if t.AccessToken == "" {
		return ErrEmptyAccessToken
	}
	var email string
	if t.Extra == nil || t.Extra["email"] == "" {
		conf, err := c.scheme.loadConfig()
		if err != nil {
			return err
		}
		transport := &goauth2.Transport{Config: &conf}
		transport.Token = t
		client := transport.Client()
		response, err := client.Get(c.scheme.InfoUrl)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		email, err = c.scheme.Parser.Parse(response)
		if email == "" {
			return ErrEmptyUserEmail
		}
		user, err := auth.GetUserByEmail(email)
		if err != nil {
			if err != auth.ErrUserNotFound {
				return err
			}
			registrationEnabled, _ := config.GetBool("auth:user-registration")
			if !registrationEnabled {
				return err
			}
			user = &auth.User{Email: email}
			err := user.Create()
			if err != nil {
				return err
			}
		}
		err = user.CreateOnGandalf()
		if err != nil {
			log.Errorf("Ignored error trying to create user on gandalf: %s", err.Error())
		}
		t.Extra = make(map[string]string)
		t.Extra["email"] = email
	}
	return makeToken(t).save()
}

func init() {
	auth.RegisterScheme("oauth", &OAuthScheme{})
}

// This method loads basic config and returns a copy of the
// config object.
func (s *OAuthScheme) loadConfig() (goauth2.Config, error) {
	if s.BaseConfig.ClientId != "" {
		return s.BaseConfig, nil
	}
	if s.Parser == nil {
		s.Parser = s
	}
	var emptyConfig goauth2.Config
	clientId, err := config.GetString("auth:oauth:client-id")
	if err != nil {
		return emptyConfig, err
	}
	clientSecret, err := config.GetString("auth:oauth:client-secret")
	if err != nil {
		return emptyConfig, err
	}
	scope, err := config.GetString("auth:oauth:scope")
	if err != nil {
		return emptyConfig, err
	}
	authURL, err := config.GetString("auth:oauth:auth-url")
	if err != nil {
		return emptyConfig, err
	}
	tokenURL, err := config.GetString("auth:oauth:token-url")
	if err != nil {
		return emptyConfig, err
	}
	infoURL, err := config.GetString("auth:oauth:info-url")
	if err != nil {
		return emptyConfig, err
	}
	callbackPort, err := config.GetInt("auth:oauth:callback-port")
	if err != nil {
		log.Debugf("auth:oauth:callback-port not found using random port: %s", err)
	}
	s.InfoUrl = infoURL
	s.CallbackPort = callbackPort
	s.BaseConfig = goauth2.Config{
		ClientId:     clientId,
		ClientSecret: clientSecret,
		Scope:        scope,
		AuthURL:      authURL,
		TokenURL:     tokenURL,
		TokenCache:   &DBTokenCache{s},
	}
	return s.BaseConfig, nil
}

func (s *OAuthScheme) Login(params map[string]string) (auth.Token, error) {
	config, err := s.loadConfig()
	if err != nil {
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
	config.RedirectURL = redirectUrl
	transport := &goauth2.Transport{Config: &config}
	oauthToken, err := transport.Exchange(code)
	if err != nil {
		return nil, err
	}
	return makeToken(oauthToken), nil
}

func (s *OAuthScheme) AppLogin(appName string) (auth.Token, error) {
	nativeScheme := native.NativeScheme{}
	return nativeScheme.AppLogin(appName)
}

func (s *OAuthScheme) Logout(token string) error {
	return deleteToken(token)
}

func (s *OAuthScheme) Auth(header string) (auth.Token, error) {
	token, err := getToken(header)
	if err != nil {
		nativeScheme := native.NativeScheme{}
		token, nativeErr := nativeScheme.Auth(header)
		if nativeErr == nil && token.IsAppToken() {
			return token, nil
		}
		return nil, err
	}
	config, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	transport := goauth2.Transport{Config: &config}
	transport.Token = &token.Token
	client := transport.Client()
	rsp, err := client.Get(s.InfoUrl)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()
	return makeToken(transport.Token), nil
}

func (s *OAuthScheme) Name() string {
	return "oauth"
}

func (s *OAuthScheme) Info() (auth.SchemeInfo, error) {
	config, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	config.RedirectURL = "__redirect_url__"
	return auth.SchemeInfo{"authorizeUrl": config.AuthCodeURL(""), "port": strconv.Itoa(s.CallbackPort)}, nil
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

func (s *OAuthScheme) Create(user *auth.User) (*auth.User, error) {
	err := user.Create()
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *OAuthScheme) Remove(token auth.Token) error {
	u, err := token.User()
	if err != nil {
		return err
	}
	err = deleteAllTokens(u.Email)
	if err != nil {
		return err
	}
	return u.Delete()
}
