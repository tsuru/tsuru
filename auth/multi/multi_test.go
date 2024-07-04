// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package multi

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"
	authTypes "github.com/tsuru/tsuru/types/auth"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type MultiSuite struct{}

var _ = check.Suite(&MultiSuite{})

func (s *MultiSuite) TestLogin(c *check.C) {
	type testCase struct {
		desc          string
		schemes       []auth.Scheme
		expectedError string
		expectedToken string
	}

	testCases := []testCase{
		{
			desc: "fail on last scheme",
			schemes: []auth.Scheme{
				&fakeScheme{
					login: func(params map[string]string) (auth.Token, error) {
						return nil, nil
					},
				},
				&fakeScheme{
					login: func(params map[string]string) (auth.Token, error) {
						return nil, errors.New("failed to login on scheme02")
					},
				},
			},
			expectedError: "failed to login on scheme02",
		},
		{
			desc: "fail on all schemes",
			schemes: []auth.Scheme{
				&fakeScheme{
					login: func(params map[string]string) (auth.Token, error) {
						return nil, errors.New("failed to login on scheme01")
					},
				},
				&fakeScheme{
					login: func(params map[string]string) (auth.Token, error) {
						return nil, errors.New("failed to login on scheme02")
					},
				},
			},
			expectedError: `multiple errors reported (2):
error #0: failed to login on scheme01
error #1: failed to login on scheme02
`,
		},
		{
			desc: "pass on first scheme",
			schemes: []auth.Scheme{
				&fakeScheme{
					login: func(params map[string]string) (auth.Token, error) {
						return fakeToken("token01"), nil
					},
				},
				&fakeScheme{
					login: func(params map[string]string) (auth.Token, error) {
						return nil, errors.New("failed to login on scheme02")
					},
				},
			},
			expectedToken: "token01",
		},
		{
			desc: "pass on last scheme",
			schemes: []auth.Scheme{
				&fakeScheme{
					login: func(params map[string]string) (auth.Token, error) {
						return nil, errors.New("failed to login on scheme01")
					},
				},
				&fakeScheme{
					login: func(params map[string]string) (auth.Token, error) {
						return fakeToken("token02"), nil
					},
				},
			},
			expectedToken: "token02",
		},
		{
			desc: "no schemes implements login",
			schemes: []auth.Scheme{
				&fakeScheme{
					login: func(params map[string]string) (auth.Token, error) {
						return nil, nil
					},
				},
				&fakeScheme{
					login: func(params map[string]string) (auth.Token, error) {
						return nil, nil
					},
				},
			},
			expectedError: "login is not implemented by any schemes",
		},
	}

	for _, testCase := range testCases {
		scheme := &multiScheme{
			cachedSchemes: atomic.Pointer[[]auth.Scheme]{},
		}
		scheme.cachedSchemes.Store(&testCase.schemes)
		token, err := scheme.Login(context.TODO(), map[string]string{})

		if testCase.expectedError != "" {
			c.Check(err, check.Not(check.IsNil))
			if c.Check(token, check.IsNil) {
				c.Check(err.Error(), check.Equals, testCase.expectedError)
			}
		}

		if testCase.expectedToken != "" {
			c.Check(err, check.IsNil)
			if c.Check(token, check.Not(check.IsNil)) {
				c.Assert(token.GetValue(), check.Equals, testCase.expectedToken)
			}
		}
	}

}

func (s *MultiSuite) TestInfos(c *check.C) {
	type testCase struct {
		desc          string
		schemes       []auth.Scheme
		defaultScheme string
		expectedError string
		expectedInfos []authTypes.SchemeInfo
	}

	testCases := []testCase{
		{
			desc: "no defaults",
			schemes: []auth.Scheme{
				&fakeScheme{
					info: func() (*authTypes.SchemeInfo, error) {
						return &authTypes.SchemeInfo{Name: "auth1"}, nil
					},
				},
				&fakeScheme{
					info: func() (*authTypes.SchemeInfo, error) {
						return &authTypes.SchemeInfo{Name: "auth2"}, nil
					},
				},
			},
			expectedInfos: []authTypes.SchemeInfo{
				{Name: "auth1", Default: true},
				{Name: "auth2"},
			},
		},

		{
			desc:          "default defined by config",
			defaultScheme: "auth2",
			schemes: []auth.Scheme{
				&fakeScheme{
					info: func() (*authTypes.SchemeInfo, error) {
						return &authTypes.SchemeInfo{Name: "auth1"}, nil
					},
				},
				&fakeScheme{
					info: func() (*authTypes.SchemeInfo, error) {
						return &authTypes.SchemeInfo{Name: "auth2"}, nil
					},
				},
			},
			expectedInfos: []authTypes.SchemeInfo{
				{Name: "auth1"},
				{Name: "auth2", Default: true},
			},
		},
	}

	for _, testCase := range testCases {
		scheme := &multiScheme{
			cachedSchemes: atomic.Pointer[[]auth.Scheme]{},
		}
		scheme.cachedSchemes.Store(&testCase.schemes)
		config.Set("auth:multi:default-scheme", testCase.defaultScheme)
		infos, err := scheme.Infos(context.TODO())

		if testCase.expectedError != "" {
			c.Check(err, check.Not(check.IsNil))
			if c.Check(infos, check.IsNil) {
				c.Check(err.Error(), check.Equals, testCase.expectedError)
			}
		}

		c.Assert(infos, check.DeepEquals, testCase.expectedInfos)
	}

}

func (s *MultiSuite) TestLogout(c *check.C) {
	type testCase struct {
		desc          string
		schemes       []auth.Scheme
		expectedError string
	}

	testCases := []testCase{
		{
			desc: "fail on last scheme",
			schemes: []auth.Scheme{
				&fakeScheme{
					logout: func(token string) error {
						return nil
					},
				},
				&fakeScheme{
					logout: func(token string) error {
						return errors.New("failed to logout on scheme02")
					},
				},
			},
			expectedError: "",
		},
		{
			desc: "fail on all schemes",
			schemes: []auth.Scheme{
				&fakeScheme{
					logout: func(token string) error {
						return errors.New("failed to logout on scheme01")
					},
				},
				&fakeScheme{
					logout: func(token string) error {
						return errors.New("failed to logout on scheme02")
					},
				},
			},
			expectedError: `multiple errors reported (2):
error #0: failed to logout on scheme01
error #1: failed to logout on scheme02
`,
		},
		{
			desc: "pass on first scheme",
			schemes: []auth.Scheme{
				&fakeScheme{
					logout: func(token string) error {
						return nil
					},
				},
				&fakeScheme{
					logout: func(token string) error {
						return errors.New("failed to logout on scheme02")
					},
				},
			},
			expectedError: "",
		},
		{
			desc: "pass on last scheme",
			schemes: []auth.Scheme{
				&fakeScheme{
					logout: func(token string) error {
						return errors.New("failed to logout on scheme01")
					},
				},
				&fakeScheme{
					logout: func(token string) error {
						return nil
					},
				},
			},
			expectedError: "",
		},
		{
			desc: "no schemes implements logout",
			schemes: []auth.Scheme{
				&fakeScheme{
					logout: func(token string) error {
						return nil
					},
				},
				&fakeScheme{
					logout: func(token string) error {
						return nil
					},
				},
			},
			expectedError: "",
		},
	}

	for _, testCase := range testCases {
		scheme := &multiScheme{
			cachedSchemes: atomic.Pointer[[]auth.Scheme]{},
		}
		scheme.cachedSchemes.Store(&testCase.schemes)
		err := scheme.Logout(context.TODO(), "faketoken")

		if testCase.expectedError == "" {
			c.Check(err, check.IsNil)
		} else {
			if c.Check(err, check.NotNil) {
				c.Check(err.Error(), check.Equals, testCase.expectedError)
			}
		}
	}
}

func (s *MultiSuite) TestAuth(c *check.C) {
	type testCase struct {
		desc          string
		schemes       []auth.Scheme
		expectedError string
		expectedToken string
	}

	testCases := []testCase{
		{
			desc: "fail on last scheme",
			schemes: []auth.Scheme{
				&fakeScheme{
					auth: func(token string) (auth.Token, error) {
						return nil, nil
					},
				},
				&fakeScheme{
					auth: func(token string) (auth.Token, error) {
						return nil, errors.New("failed to auth on scheme02")
					},
				},
			},
			expectedError: "failed to auth on scheme02",
		},
		{
			desc: "fail on all schemes",
			schemes: []auth.Scheme{
				&fakeScheme{
					auth: func(token string) (auth.Token, error) {
						return nil, errors.New("failed to auth on scheme01")
					},
				},
				&fakeScheme{
					auth: func(token string) (auth.Token, error) {
						return nil, errors.New("failed to auth on scheme02")
					},
				},
			},
			expectedError: `multiple errors reported (2):
error #0: failed to auth on scheme01
error #1: failed to auth on scheme02
`,
		},
		{
			desc: "fail by invalid token on all schemes",
			schemes: []auth.Scheme{
				&fakeScheme{
					auth: func(token string) (auth.Token, error) {
						return nil, auth.ErrInvalidToken
					},
				},
				&fakeScheme{
					auth: func(token string) (auth.Token, error) {
						return nil, auth.ErrInvalidToken
					},
				},
			},
			expectedError: auth.ErrInvalidToken.Error(),
		},
		{
			desc: "pass on first scheme",
			schemes: []auth.Scheme{
				&fakeScheme{
					auth: func(token string) (auth.Token, error) {
						return fakeToken("token01"), nil
					},
				},
				&fakeScheme{
					auth: func(token string) (auth.Token, error) {
						return nil, errors.New("failed to auth on scheme02")
					},
				},
			},
			expectedToken: "token01",
		},
		{
			desc: "pass on last scheme",
			schemes: []auth.Scheme{
				&fakeScheme{
					auth: func(token string) (auth.Token, error) {
						return nil, errors.New("failed to auth on scheme01")
					},
				},
				&fakeScheme{
					auth: func(token string) (auth.Token, error) {
						return fakeToken("token02"), nil
					},
				},
			},
			expectedToken: "token02",
		},
		{
			desc: "no schemes implements auth",
			schemes: []auth.Scheme{
				&fakeScheme{
					auth: func(token string) (auth.Token, error) {
						return nil, nil
					},
				},
				&fakeScheme{
					auth: func(token string) (auth.Token, error) {
						return nil, nil
					},
				},
			},
			expectedError: "auth is not implemented by any schemes",
		},
	}

	for _, testCase := range testCases {
		scheme := &multiScheme{
			cachedSchemes: atomic.Pointer[[]auth.Scheme]{},
		}
		scheme.cachedSchemes.Store(&testCase.schemes)
		token, err := scheme.Auth(context.TODO(), "token")

		if testCase.expectedError != "" {
			c.Assert(err, check.NotNil)
			c.Assert(token, check.IsNil)
			c.Assert(err.Error(), check.Equals, testCase.expectedError)
		}

		if testCase.expectedToken != "" {
			c.Check(err, check.IsNil)
			if c.Check(token, check.NotNil) {
				c.Check(token.GetValue(), check.Equals, testCase.expectedToken)
			}
		}
	}

}

func (s *MultiSuite) TestCreate(c *check.C) {
	type testCase struct {
		desc          string
		schemes       []auth.Scheme
		expectedError string
		expectedUser  string
	}

	testCases := []testCase{
		{
			desc: "fail on last scheme",
			schemes: []auth.Scheme{
				&fakeScheme{
					create: func(u *auth.User) (*auth.User, error) {
						return nil, nil
					},
				},
				&fakeScheme{
					create: func(u *auth.User) (*auth.User, error) {
						return nil, errors.New("failed to create on scheme02")
					},
				},
			},
			expectedError: "failed to create on scheme02",
		},
		{
			desc: "fail on all schemes",
			schemes: []auth.Scheme{
				&fakeScheme{
					create: func(u *auth.User) (*auth.User, error) {
						return nil, errors.New("failed to create on scheme01")
					},
				},
				&fakeScheme{
					create: func(u *auth.User) (*auth.User, error) {
						return nil, errors.New("failed to create on scheme02")
					},
				},
			},
			expectedError: `multiple errors reported (2):
error #0: failed to create on scheme01
error #1: failed to create on scheme02
`,
		},
		{
			desc: "pass on first scheme",
			schemes: []auth.Scheme{
				&fakeScheme{
					create: func(u *auth.User) (*auth.User, error) {
						return &auth.User{Email: "blah"}, nil
					},
				},
				&fakeScheme{
					create: func(u *auth.User) (*auth.User, error) {
						return nil, errors.New("failed to create on scheme02")
					},
				},
			},
			expectedUser: "blah",
		},
		{
			desc: "pass on last scheme",
			schemes: []auth.Scheme{
				&fakeScheme{
					create: func(u *auth.User) (*auth.User, error) {
						return nil, errors.New("failed to create on scheme01")
					},
				},
				&fakeScheme{
					create: func(u *auth.User) (*auth.User, error) {
						return &auth.User{Email: "blah"}, nil
					},
				},
			},
			expectedUser: "blah",
		},
		{
			desc: "no schemes implements create",
			schemes: []auth.Scheme{
				&fakeScheme{
					create: func(u *auth.User) (*auth.User, error) {
						return nil, nil
					},
				},
				&fakeScheme{
					create: func(u *auth.User) (*auth.User, error) {
						return nil, nil
					},
				},
			},
			expectedError: "create is not implemented by any schemes",
		},
	}

	for _, testCase := range testCases {
		scheme := &multiScheme{
			cachedSchemes: atomic.Pointer[[]auth.Scheme]{},
		}
		scheme.cachedSchemes.Store(&testCase.schemes)
		user, err := scheme.Create(context.TODO(), &auth.User{})

		if testCase.expectedError != "" {
			c.Assert(err, check.NotNil)
			c.Assert(user, check.IsNil)
			c.Assert(err.Error(), check.Equals, testCase.expectedError)
		}

		if testCase.expectedUser != "" {
			c.Check(err, check.IsNil)
			if c.Check(user, check.NotNil) {
				c.Check(user.Email, check.Equals, testCase.expectedUser)
			}
		}
	}
}

type fakeScheme struct {
	login  func(params map[string]string) (auth.Token, error)
	logout func(token string) error
	auth   func(token string) (auth.Token, error)
	info   func() (*authTypes.SchemeInfo, error)
	create func(u *auth.User) (*auth.User, error)
	remove func(u *auth.User) error
}

func (t *fakeScheme) Login(ctx context.Context, params map[string]string) (auth.Token, error) {
	if t.login != nil {
		return t.login(params)
	}
	return nil, nil
}
func (t *fakeScheme) Logout(ctx context.Context, token string) error {
	if t.logout != nil {
		return t.logout(token)
	}
	return nil
}
func (t *fakeScheme) Auth(ctx context.Context, token string) (auth.Token, error) {
	if t.auth != nil {
		return t.auth(token)
	}
	return nil, nil
}
func (t *fakeScheme) Info(ctx context.Context) (*authTypes.SchemeInfo, error) {
	if t.info != nil {
		return t.info()
	}
	return nil, nil
}
func (t *fakeScheme) Name() string {
	return "test"
}
func (t *fakeScheme) Create(ctx context.Context, u *auth.User) (*auth.User, error) {
	if t.create != nil {
		return t.create(u)
	}
	return nil, nil
}
func (t *fakeScheme) Remove(ctx context.Context, u *auth.User) error {
	if t.remove != nil {
		return t.remove(u)
	}
	return nil
}

type fakeToken string

func (t fakeToken) GetValue() string {
	return string(t)
}

func (t fakeToken) User() (*authTypes.User, error) {
	return &authTypes.User{}, nil
}

func (t fakeToken) GetUserName() string {
	return "faketoken"
}

func (t fakeToken) Engine() string {
	return "fake"
}

func (t fakeToken) Permissions(ctx context.Context) ([]permission.Permission, error) {
	return []permission.Permission{}, nil
}
