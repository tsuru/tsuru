// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package oauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/set"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"golang.org/x/oauth2"
)

var (
	ErrMissingCodeError       = &tsuruErrors.ValidationError{Message: "You must provide code to login"}
	ErrMissingCodeRedirectURL = &tsuruErrors.ValidationError{Message: "You must provide the used redirect url to login"}
	ErrEmptyAccessToken       = &tsuruErrors.NotAuthorizedError{Message: "Couldn't convert code to access token."}
	ErrEmptyUserEmail         = &tsuruErrors.NotAuthorizedError{Message: "Couldn't parse user email."}

	requestLatencies = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "tsuru_oauth_request_duration_seconds",
		Help: "The oauth requests latency distributions.",
	})
	requestErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tsuru_oauth_request_errors_total",
		Help: "The total number of oauth request errors.",
	})

	_ auth.Scheme     = &oAuthScheme{}
	_ auth.UserScheme = &oAuthScheme{}
)

type oAuthScheme struct {
	infoURL      string
	callbackPort int
}

func init() {
	auth.RegisterScheme("oauth", &oAuthScheme{})
	prometheus.MustRegister(requestLatencies)
	prometheus.MustRegister(requestErrors)
}

// This method loads basic config and returns a copy of the
// config object.
func (s *oAuthScheme) loadConfig() (oauth2.Config, error) {
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
	s.infoURL = infoURL
	s.callbackPort = callbackPort
	return oauth2.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		Scopes:       []string{scope},
		Endpoint: oauth2.Endpoint{
			AuthURL:  authURL,
			TokenURL: tokenURL,
		},
	}, nil
}

func (s *oAuthScheme) Login(ctx context.Context, params map[string]string) (auth.Token, error) {
	conf, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	code, ok := params["code"]
	if !ok {
		return nil, ErrMissingCodeError
	}
	redirectURL, ok := params["redirectUrl"]
	if !ok {
		return nil, ErrMissingCodeRedirectURL
	}
	conf.RedirectURL = redirectURL
	tracedClientCtx := context.WithValue(ctx, oauth2.HTTPClient, tsuruNet.Dial15Full60ClientWithPool)
	oauthToken, err := conf.Exchange(tracedClientCtx, code)
	if err != nil {
		return nil, err
	}
	return s.handleToken(ctx, oauthToken)
}

func (s *oAuthScheme) handleToken(ctx context.Context, t *oauth2.Token) (*tokenWrapper, error) {
	if t.AccessToken == "" {
		return nil, ErrEmptyAccessToken
	}
	conf, err := s.loadConfig()
	if err != nil {
		return nil, err
	}

	tracedClientCtx := context.WithValue(context.Background(), oauth2.HTTPClient, tsuruNet.Dial15Full60ClientWithPool)

	client := conf.Client(tracedClientCtx, t)
	t0 := time.Now()

	req, err := http.NewRequest(http.MethodGet, s.infoURL, nil)
	if err != nil {
		requestErrors.Inc()
		return nil, err
	}
	req = req.WithContext(ctx)
	response, err := client.Do(req)
	requestLatencies.Observe(time.Since(t0).Seconds())
	if err != nil {
		requestErrors.Inc()
		return nil, err
	}
	defer response.Body.Close()
	user, err := s.parse(response)
	if err != nil {
		return nil, err
	}
	if user.Email == "" {
		return nil, ErrEmptyUserEmail
	}
	dbUser, err := auth.GetUserByEmail(ctx, user.Email)
	if err != nil {
		if err != authTypes.ErrUserNotFound {
			return nil, err
		}
		registrationEnabled, _ := config.GetBool("auth:user-registration")
		if !registrationEnabled {
			return nil, err
		}
		dbUser = &auth.User{Email: user.Email}
		err = dbUser.Create(ctx)
	} else {
		dbGroups := set.FromSlice(dbUser.Groups)
		providerGroups := set.FromSlice(user.Groups)
		if !dbGroups.Equal(providerGroups) {
			dbUser.Groups = user.Groups
			err = dbUser.Update(ctx)
		}
	}
	if err != nil {
		return nil, err
	}
	token := tokenWrapper{Token: *t, UserEmail: user.Email}
	err = token.save()
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (s *oAuthScheme) Logout(ctx context.Context, token string) error {
	return deleteToken(token)
}

func (s *oAuthScheme) Auth(ctx context.Context, header string) (auth.Token, error) {
	token, err := getToken(header)
	if err != nil {
		return nil, err
	}
	if !token.Token.Valid() {
		return token, auth.ErrInvalidToken
	}
	return token, nil
}

func (s *oAuthScheme) Info(ctx context.Context) (*authTypes.SchemeInfo, error) {
	config, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	config.RedirectURL = "__redirect_url__"
	return &authTypes.SchemeInfo{
		Name: "oauth",
		Data: authTypes.SchemeData{
			AuthorizeURL: config.AuthCodeURL(""),
			Port:         strconv.Itoa(s.callbackPort),
		},
	}, nil
}

type userData struct {
	Email  string   `json:"email"`
	Groups []string `json:"groups"`
}

func (s *oAuthScheme) parse(infoResponse *http.Response) (userData, error) {
	var user userData
	data, err := io.ReadAll(infoResponse.Body)
	if err != nil {
		return user, errors.Wrap(err, "unable to read user data response")
	}
	if infoResponse.StatusCode != http.StatusOK {
		return user, errors.Errorf("unexpected user data response %d: %s", infoResponse.StatusCode, data)
	}
	err = json.Unmarshal(data, &user)
	if err != nil {
		return user, errors.Wrapf(err, "unable to parse user data: %s", data)
	}
	return user, nil
}

func (s *oAuthScheme) Create(ctx context.Context, user *auth.User) (*auth.User, error) {
	user.Password = ""
	err := user.Create(ctx)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *oAuthScheme) Remove(ctx context.Context, u *auth.User) error {
	err := deleteAllTokens(u.Email)
	if err != nil {
		return err
	}
	return u.Delete(ctx)
}
