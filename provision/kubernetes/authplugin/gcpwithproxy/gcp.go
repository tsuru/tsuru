/*
Copyright 2016 The Kubernetes Authors.
Copyright 2021 Tsuru authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gcpwithproxy

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"k8s.io/apimachinery/pkg/util/net"
	restclient "k8s.io/client-go/rest"
)

func init() {
	if err := restclient.RegisterAuthProviderPlugin("gcp-with-proxy", newGCPAuthProvider); err != nil {
		log.Fatalf("Failed to register gcp auth plugin: %v", err)
	}
}

var (

	// defaultScopes:
	// - cloud-platform is the base scope to authenticate to GCP.
	// - userinfo.email is used to authenticate to GKE APIs with gserviceaccount
	//   email instead of numeric uniqueID.
	defaultScopes = []string{
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/userinfo.email"}
)

// gcpAuthProvider is an auth provider plugin that uses GCP credentials to provide
// tokens for kubectl to authenticate itself to the apiserver. A sample json config
// is provided below with all recognized options described.
//
// {
//   'auth-provider': {
//     # Required
//     "name": "gcp",
//
//     'config': {
//       # Authentication options
//       # These options are used while getting a token.
//
//       # comma-separated list of GCP API scopes. default value of this field
//       # is "https://www.googleapis.com/auth/cloud-platform,https://www.googleapis.com/auth/userinfo.email".
// 		 # to override the API scopes, specify this field explicitly.
//       "scopes": "https://www.googleapis.com/auth/cloud-platform"
//
//       # Caching options
//
//       # Raw string data representing cached access token.
//       "access-token": "ya29.CjWdA4GiBPTt",
//       # RFC3339Nano expiration timestamp for cached access token.
//       "expiry": "2016-10-31 22:31:9.123",
//
//       # Command execution options
//       # These options direct the plugin to execute a specified command and parse
//       # token and expiry time from the output of the command.
//
//     }
//   }
// }
//
type gcpAuthProvider struct {
	tokenSource oauth2.TokenSource
	persister   restclient.AuthProviderConfigPersister
	hasProxy    bool
}

func newGCPAuthProvider(_ string, gcpConfig map[string]string, persister restclient.AuthProviderConfigPersister) (restclient.AuthProvider, error) {
	explicitProxy := gcpConfig["http-proxy"] != ""
	ts, err := tokenSource(gcpConfig)
	if err != nil {
		return nil, err
	}
	cts, err := newCachedTokenSource(gcpConfig["access-token"], gcpConfig["expiry"], persister, ts, gcpConfig)
	if err != nil {
		return nil, err
	}
	return &gcpAuthProvider{cts, persister, explicitProxy}, nil
}

func tokenSource(gcpConfig map[string]string) (oauth2.TokenSource, error) {
	if gcpConfig["dry-run"] != "" {
		return &dryRunTokenSource{
			token: &oauth2.Token{
				AccessToken: "my-fake-token",
			},
		}, nil
	}
	// Google Application Credentials-based token source
	scopes := parseScopes(gcpConfig)

	httpProxy := gcpConfig["http-proxy"]
	ctx := context.Background()
	client := tsuruNet.Dial15Full60ClientNoKeepAlive
	if httpProxy != "" {
		var err error
		client, err = tsuruNet.WithProxy(*client, httpProxy)
		if err != nil {
			return nil, err
		}
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, client)

	ts, err := google.DefaultTokenSource(ctx, scopes...)
	if err != nil {
		return nil, fmt.Errorf("cannot construct google default token source: %v", err)
	}
	return ts, nil
}

// parseScopes constructs a list of scopes that should be included in token source
// from the config map.
func parseScopes(gcpConfig map[string]string) []string {
	scopes, ok := gcpConfig["scopes"]
	if !ok {
		return defaultScopes
	}
	if scopes == "" {
		return []string{}
	}
	return strings.Split(gcpConfig["scopes"], ",")
}

func (g *gcpAuthProvider) WrapTransport(rt http.RoundTripper) http.RoundTripper {
	var resetCache map[string]string
	if cts, ok := g.tokenSource.(*cachedTokenSource); ok {
		resetCache = cts.baseCache()
	} else {
		resetCache = make(map[string]string)
	}
	return &conditionalTransport{
		oauthTransport: &oauth2.Transport{Source: g.tokenSource, Base: rt},
		persister:      g.persister,
		resetCache:     resetCache,
		hasProxy:       g.hasProxy,
	}
}

func (g *gcpAuthProvider) Login() error { return nil }

type cachedTokenSource struct {
	lk          sync.Mutex
	source      oauth2.TokenSource
	accessToken string `datapolicy:"token"`
	expiry      time.Time
	persister   restclient.AuthProviderConfigPersister
	cache       map[string]string
}

func newCachedTokenSource(accessToken, expiry string, persister restclient.AuthProviderConfigPersister, ts oauth2.TokenSource, cache map[string]string) (*cachedTokenSource, error) {
	var expiryTime time.Time
	if parsedTime, err := time.Parse(time.RFC3339Nano, expiry); err == nil {
		expiryTime = parsedTime
	}
	if cache == nil {
		cache = make(map[string]string)
	}
	return &cachedTokenSource{
		source:      ts,
		accessToken: accessToken,
		expiry:      expiryTime,
		persister:   persister,
		cache:       cache,
	}, nil
}

func (t *cachedTokenSource) Token() (*oauth2.Token, error) {
	tok := t.cachedToken()
	if tok.Valid() && !tok.Expiry.IsZero() {
		return tok, nil
	}
	tok, err := t.source.Token()
	if err != nil {
		return nil, err
	}
	cache := t.update(tok)
	if t.persister != nil {
		if err := t.persister.Persist(cache); err != nil {
			log.Debugf("Failed to persist token: %v", err)
		}
	}
	return tok, nil
}

func (t *cachedTokenSource) cachedToken() *oauth2.Token {
	t.lk.Lock()
	defer t.lk.Unlock()
	return &oauth2.Token{
		AccessToken: t.accessToken,
		TokenType:   "Bearer",
		Expiry:      t.expiry,
	}
}

func (t *cachedTokenSource) update(tok *oauth2.Token) map[string]string {
	t.lk.Lock()
	defer t.lk.Unlock()
	t.accessToken = tok.AccessToken
	t.expiry = tok.Expiry
	ret := map[string]string{}
	for k, v := range t.cache {
		ret[k] = v
	}
	ret["access-token"] = t.accessToken
	ret["expiry"] = t.expiry.Format(time.RFC3339Nano)
	return ret
}

// baseCache is the base configuration value for this TokenSource, without any cached ephemeral tokens.
func (t *cachedTokenSource) baseCache() map[string]string {
	t.lk.Lock()
	defer t.lk.Unlock()
	ret := map[string]string{}
	for k, v := range t.cache {
		ret[k] = v
	}
	delete(ret, "access-token")
	delete(ret, "expiry")
	return ret
}

type conditionalTransport struct {
	oauthTransport *oauth2.Transport
	persister      restclient.AuthProviderConfigPersister
	resetCache     map[string]string
	hasProxy       bool
}

var _ net.RoundTripperWrapper = &conditionalTransport{}

func (t *conditionalTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	transport := *t.oauthTransport
	if !t.hasProxy {
		newCli, err := tsuruNet.WithProxyFromConfig(http.Client{Transport: transport.Base}, req.URL.Host)
		if err != nil {
			return nil, err
		}
		transport.Base = newCli.Transport
	}

	if len(req.Header.Get("Authorization")) != 0 {
		return transport.Base.RoundTrip(req)
	}

	res, err := transport.RoundTrip(req)

	if err != nil {
		return nil, err
	}

	if res.StatusCode == 401 {
		log.Debug("The credentials that were supplied are invalid for the target cluster")
		t.persister.Persist(t.resetCache)
	}

	return res, nil
}

func (t *conditionalTransport) WrappedRoundTripper() http.RoundTripper { return t.oauthTransport.Base }

type dryRunTokenSource struct{ token *oauth2.Token }

func (t *dryRunTokenSource) Token() (*oauth2.Token, error) {
	return t.token, nil
}
