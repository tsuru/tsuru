package ldap

import (
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/errors"
)

var (
	ErrNotImplementedLdap = &errors.ValidationError{Message: "this function wasn't implemented with LDAP authentication"}
)

type LdapNativeScheme struct {
	native.NativeScheme
}

func init() {
	auth.RegisterScheme("ldap", LdapNativeScheme{})
}

func (s LdapNativeScheme) Login(params map[string]string) (auth.Token, error) {
	email, ok := params["email"]
	if !ok {
		return nil, native.ErrMissingEmailError
	}
	password, ok := params["password"]
	if !ok {
		return nil, native.ErrMissingPasswordError
	}
	user, err := auth.GetUserByEmail(email)
	if err != nil {
		return nil, err
	}
	token, err := createToken(user, password)
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (s LdapNativeScheme) ChangePassword(token auth.Token, oldPassword string, newPassword string) error {
	return ErrNotImplementedLdap
}

func (s LdapNativeScheme) StartPasswordReset(user *auth.User) error {
	return ErrNotImplementedLdap
}

func (s LdapNativeScheme) ResetPassword(user *auth.User, resetToken string) error {
	return ErrNotImplementedLdap
}

func (s LdapNativeScheme) Name() string {
	return "ldap"
}

func (s LdapNativeScheme) Info() (auth.SchemeInfo, error) {
	return nil, nil
}
