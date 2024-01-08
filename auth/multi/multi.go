// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package multi

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	tsuruErrors "github.com/tsuru/tsuru/errors"
)

var (
	_ auth.Scheme      = &multiScheme{}
	_ auth.MultiScheme = &multiScheme{}
)

func init() {
	auth.RegisterScheme("multi", &multiScheme{
		cachedSchemes: atomic.Pointer[[]auth.Scheme]{},
	})
}

type multiScheme struct {
	cachedSchemes atomic.Pointer[[]auth.Scheme]
}

func (s *multiScheme) Name() string {
	return "multi"
}

func (s *multiScheme) Login(ctx context.Context, params map[string]string) (auth.Token, error) {
	schemes, err := s.schemes()
	if err != nil {
		return nil, err
	}

	errors := tsuruErrors.NewMultiError()
	for _, scheme := range schemes {
		userScheme, ok := scheme.(auth.UserScheme)
		if !ok {
			continue
		}
		authToken, err := userScheme.Login(ctx, params)

		if err != nil {
			errors.Add(err)
			continue
		}

		if authToken != nil {
			return authToken, nil
		}
	}

	if errors.Len() > 0 {
		return nil, errors.ToError()
	}

	return nil, newErrNotImplemented("login")
}

func (s *multiScheme) Logout(ctx context.Context, token string) error {
	schemes, err := s.schemes()
	if err != nil {
		return err
	}

	errors := tsuruErrors.NewMultiError()
	for _, scheme := range schemes {
		userScheme, ok := scheme.(auth.UserScheme)
		if !ok {
			continue
		}
		err := userScheme.Logout(ctx, token)

		if err != nil {
			errors.Add(err)
			continue
		}

		return nil
	}

	if errors.Len() > 0 {
		return errors.ToError()
	}

	return newErrNotImplemented("logout")
}

func (s *multiScheme) Auth(ctx context.Context, token string) (auth.Token, error) {
	schemes, err := s.schemes()
	if err != nil {
		return nil, err
	}

	var tokenInvalidCount int

	errors := tsuruErrors.NewMultiError()
	for _, scheme := range schemes {
		authToken, err := scheme.Auth(ctx, token)

		if err == auth.ErrInvalidToken {
			tokenInvalidCount++
			continue
		}

		if err != nil {
			errors.Add(err)
			continue
		}

		if authToken != nil {
			return authToken, nil
		}
	}

	if tokenInvalidCount > 0 && errors.Len() == 0 {
		return nil, auth.ErrInvalidToken
	}

	if errors.Len() > 0 {
		return nil, errors.ToError()
	}

	return nil, newErrNotImplemented("auth")
}

func (s *multiScheme) Info(ctx context.Context) (*auth.SchemeInfo, error) {
	schemes, err := s.schemes()
	if err != nil {
		return nil, err
	}

	errors := tsuruErrors.NewMultiError()
	for _, scheme := range schemes {
		// for compatibility reasons the method info must return first authScheme that implements auth.UserScheme
		// previously auth.Scheme had all methods of auth.UserScheme, for this reason we prefer to maintain interoperability with old clients
		if _, ok := scheme.(auth.UserScheme); !ok {
			continue
		}

		schemeInfo, err := scheme.Info(ctx)

		if err != nil {
			errors.Add(err)
			continue
		}

		if schemeInfo != nil {
			return schemeInfo, nil
		}
	}

	if errors.Len() > 0 {
		return nil, errors.ToError()
	}

	return nil, newErrNotImplemented("info")
}

func (s *multiScheme) Infos(ctx context.Context) ([]auth.SchemeInfo, error) {
	schemes, err := s.schemes()
	if err != nil {
		return nil, err
	}

	infos := []auth.SchemeInfo{}
	errors := tsuruErrors.NewMultiError()
	for _, scheme := range schemes {
		schemeInfo, err := scheme.Info(ctx)

		if err != nil {
			errors.Add(err)
			continue
		}

		infos = append(infos, *schemeInfo)
	}

	if errors.Len() > 0 {
		return nil, errors.ToError()
	}

	return infos, nil
}

func (s *multiScheme) Create(ctx context.Context, user *auth.User) (*auth.User, error) {
	schemes, err := s.schemes()
	if err != nil {
		return nil, err
	}

	errors := tsuruErrors.NewMultiError()
	for _, scheme := range schemes {
		userScheme, ok := scheme.(auth.UserScheme)
		if !ok {
			continue
		}

		authUser, err := userScheme.Create(ctx, user)

		if err != nil {
			errors.Add(err)
			continue
		}

		if authUser != nil {
			return authUser, nil
		}
	}

	if errors.Len() > 0 {
		return nil, errors.ToError()
	}

	return nil, newErrNotImplemented("create")
}

func (s *multiScheme) Remove(ctx context.Context, user *auth.User) error {
	schemes, err := s.schemes()
	if err != nil {
		return err
	}

	errors := tsuruErrors.NewMultiError()
	for _, scheme := range schemes {
		userScheme, ok := scheme.(auth.UserScheme)
		if !ok {
			continue
		}
		err := userScheme.Remove(ctx, user)
		if err != nil {
			errors.Add(err)
			continue
		}

		return nil
	}

	if errors.Len() > 0 {
		return errors.ToError()
	}

	return newErrNotImplemented("remove")
}

func (s *multiScheme) schemes() ([]auth.Scheme, error) {
	schemes := s.cachedSchemes.Load()
	if schemes == nil {
		result := []auth.Scheme{}

		schemeNames, err := config.GetList("auth:multi:schemes")
		if err != nil {
			return nil, err
		}

		for _, schemeName := range schemeNames {
			var scheme auth.Scheme
			scheme, err = auth.GetScheme(schemeName)
			if err != nil {
				return nil, err
			}

			result = append(result, scheme)
		}

		s.cachedSchemes.Store(&result)

		return result, nil
	}

	return *schemes, nil
}

type errNotImplemented struct {
	method string
}

func (e *errNotImplemented) Error() string {
	return fmt.Sprintf("%s is not implemented by any schemes", e.method)
}

func newErrNotImplemented(method string) error {
	return &errNotImplemented{method: method}
}
