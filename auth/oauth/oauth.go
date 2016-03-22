// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
)

var (
	ErrMissingCodeError       = &errors.ValidationError{Message: "You must provide code to login"}
	ErrMissingCodeRedirectUrl = &errors.ValidationError{Message: "You must provide the used redirect url to login"}
	ErrEmptyAccessToken       = &errors.NotAuthorizedError{Message: "Couldn't convert code to access token."}
	ErrEmptyUserEmail         = &errors.NotAuthorizedError{Message: "Couldn't parse user email."}
)

type OAuthParser interface {
	Parse(infoResponse *http.Response) (string, error)
}

type OAuthScheme struct {
	BaseConfig   oauth2.Config
	InfoUrl      string
	CallbackPort int
	Parser       OAuthParser
}

func init() {
	auth.RegisterScheme("oauth", &OAuthScheme{})
}

// This method loads basic config and returns a copy of the
// config object.
func (s *OAuthScheme) loadConfig() (oauth2.Config, error) {
	if s.BaseConfig.ClientID != "" {
		return s.BaseConfig, nil
	}
	if s.Parser == nil {
		s.Parser = s
	}
	var emptyConfig oauth2.Config
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
	s.BaseConfig = oauth2.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		Scopes:       []string{scope},
		Endpoint: oauth2.Endpoint{
			AuthURL:  authURL,
			TokenURL: tokenURL,
		},
	}
	return s.BaseConfig, nil
}

func (s *OAuthScheme) Login(params map[string]string) (auth.Token, error) {
	conf, err := s.loadConfig()
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
	conf.RedirectURL = redirectUrl
	oauthToken, err := conf.Exchange(context.Background(), code)
	if err != nil {
		return nil, err
	}
	return s.handleToken(oauthToken)
}

func (s *OAuthScheme) handleToken(t *oauth2.Token) (*Token, error) {
	if t.AccessToken == "" {
		return nil, ErrEmptyAccessToken
	}
	conf, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	client := conf.Client(context.Background(), t)
	response, err := client.Get(s.InfoUrl)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	email, err := s.Parser.Parse(response)
	if err != nil {
		return nil, err
	}
	if email == "" {
		return nil, ErrEmptyUserEmail
	}
	user, err := auth.GetUserByEmail(email)
	if err != nil {
		if err != auth.ErrUserNotFound {
			return nil, err
		}
		registrationEnabled, _ := config.GetBool("auth:user-registration")
		if !registrationEnabled {
			return nil, err
		}
		user = &auth.User{Email: email}
		err = user.Create()
		if err != nil {
			return nil, err
		}
	}
	token := Token{*t, email}
	err = token.save()
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (s *OAuthScheme) AppLogin(appName string) (auth.Token, error) {
	nativeScheme := native.NativeScheme{}
	return nativeScheme.AppLogin(appName)
}

func (s *OAuthScheme) AppLogout(token string) error {
	nativeScheme := native.NativeScheme{}
	return nativeScheme.AppLogout(token)
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
	client := config.Client(context.Background(), &token.Token)
	rsp, err := client.Get(s.InfoUrl)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()
	return token, nil
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
	data, err := ioutil.ReadAll(infoResponse.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read user data response: %s", err)
	}
	if infoResponse.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected user data response %d: %s", infoResponse.StatusCode, data)
	}
	err = json.Unmarshal(data, &user)
	if err != nil {
		return "", fmt.Errorf("unable to parse user data: %s - %s", data, err)
	}
	return user.Email, nil
}

func (s *OAuthScheme) Create(user *auth.User) (*auth.User, error) {
	user.Password = ""
	err := user.Create()
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *OAuthScheme) Remove(u *auth.User) error {
	err := deleteAllTokens(u.Email)
	if err != nil {
		return err
	}
	return u.Delete()
}
