package ldap

import (
	"github.com/globalsign/mgo/bson"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/errors"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/validation"
)

var (
	ErrMissingPasswordError = &errors.ValidationError{Message: "you must provide a password to login"}
	ErrMissingEmailError    = &errors.ValidationError{Message: "you must provide a email to login"}
	ErrInvalidEmail         = &errors.ValidationError{Message: "invalid email"}
	ErrInvalidPassword      = &errors.ValidationError{Message: "password length should be least 6 characters and at most 50 characters"}
	ErrEmailRegistered      = &errors.ConflictError{Message: "this email is already registered"}
	ErrPasswordMismatch     = &errors.NotAuthorizedError{Message: "the given password didn't match the user's current password"}
)

type LdapNativeScheme struct {
	native.NativeScheme
}

func init() {
	auth.RegisterScheme("ldap", LdapNativeScheme{})
}

func getUser(email string) (*auth.User, error) {
	var u auth.User
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	err = conn.Users().Find(bson.M{"email": email}).One(&u)
	if err != nil {
		return nil, authTypes.ErrUserNotFound
	}
	return &u, nil
}

func (s LdapNativeScheme) Login(params map[string]string) (auth.Token, error) {
	uid, ok := params["email"]
	if !ok {
		return nil, ErrMissingEmailError
	}
	password, ok := params["password"]
	if !ok {
		return nil, ErrMissingPasswordError
	}
	user, err := getUser(uid)
	if err != nil {
		return nil, err
	}
	token, err := createToken(user, password)
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (s LdapNativeScheme) Auth(token string) (auth.Token, error) {
	return getToken(token)
}

func (s LdapNativeScheme) Logout(token string) error {
	return deleteToken(token)
}

func (s LdapNativeScheme) AppLogin(appName string) (auth.Token, error) {
	return createApplicationToken(appName)
}

func (s LdapNativeScheme) AppLogout(token string) error {
	return s.Logout(token)
}

func (s LdapNativeScheme) Create(user *auth.User) (*auth.User, error) {
	if !validation.ValidateEmail(user.Email) {
		return nil, ErrInvalidEmail
	}
	if _, err := getUser(user.Email); err == nil {
		return nil, ErrEmailRegistered
	}
	if err := hashPassword(user); err != nil {
		return nil, err
	}
	if err := user.Create(); err != nil {
		return nil, err
	}
	return user, nil
}

func (s LdapNativeScheme) Remove(u *auth.User) error {
	err := deleteAllTokens(u.Email)
	if err != nil {
		return err
	}
	return u.Delete()
}

func (s LdapNativeScheme) Name() string {
	return "ldap"
}

func (s LdapNativeScheme) Info() (auth.SchemeInfo, error) {
	return nil, nil
}
