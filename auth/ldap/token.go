package ldap

import (
	"crypto"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/jtblin/go-ldap-client"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/db"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/permission"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/validation"
	"golang.org/x/crypto/bcrypt"
	"log"
)

const (
	keySize           = 32
	defaultExpiration = 7 * 24 * time.Hour
	passwordError     = "Password length should be least 6 characters and at most 50 characters."
	passwordMinLen    = 6
	passwordMaxLen    = 50
)

var (
	tokenExpire time.Duration
	cost        int
	// LDAP Wise vars
	ldapBaseDn             string
	ldapHost               string
	ldapPort               int
	ldapUseSSL             bool
	ldapSkipTLS            bool
	ldapInsecureSkipVerify bool
	ldapServerName         string
	ldapBindDN             string
	ldapBindPassword       string
	ldapUserFilter         string
	ldapGroupFilter        string
)

type Token struct {
	Token     string        `json:"token"`
	Creation  time.Time     `json:"creation"`
	Expires   time.Duration `json:"expires"`
	UserEmail string        `json:"email"`
	AppName   string        `json:"app"`
}

func (t *Token) GetValue() string {
	return t.Token
}

func (t *Token) User() (*authTypes.User, error) {
	return auth.ConvertOldUser(auth.GetUserByEmail(t.UserEmail))
}

func (t *Token) IsAppToken() bool {
	return t.AppName != ""
}

func (t *Token) GetUserName() string {
	return t.UserEmail
}

func (t *Token) GetAppName() string {
	return t.AppName
}

func (t *Token) Permissions() ([]permission.Permission, error) {
	return auth.BaseTokenPermission(t)
}

func loadConfig() error {
	var err error
	if cost == 0 && tokenExpire == 0 {
		var days int
		if days, err = config.GetInt("auth:token-expire-days"); err == nil {
			tokenExpire = time.Duration(int64(days) * 24 * int64(time.Hour))
		} else {
			tokenExpire = defaultExpiration
		}
		if cost, err = config.GetInt("auth:hash-cost"); err != nil {
			cost = bcrypt.DefaultCost
		}
		if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
			return errors.Errorf("Invalid value for setting %q: it must be between %d and %d.", "auth:hash-cost", bcrypt.MinCost, bcrypt.MaxCost)
		}
	}
	if ldapBaseDn, err = config.GetString("auth:ldap:basedn"); err != nil {
		ldapBaseDn = ""
	}
	if ldapHost, err = config.GetString("auth:ldap:host"); err != nil {
		return errors.Errorf("You must set LDAP authentication Hostname, in auth:ldap:host")
	}
	if ldapPort, err = config.GetInt("auth:ldap:port"); err != nil {
		ldapPort = 389
	}
	if ldapUseSSL, err = config.GetBool("auth:ldap:usessl"); err != nil {
		ldapUseSSL = false
	}
	if ldapSkipTLS, err = config.GetBool("auth:ldap:skiptls"); err != nil {
		ldapSkipTLS = false
	}
	if ldapInsecureSkipVerify, err = config.GetBool("auth:ldap:sslskipverify"); err != nil {
		ldapInsecureSkipVerify = false
	}
	if ldapServerName, err = config.GetString("auth:ldap:servername"); err != nil {
		ldapServerName = ldapHost
	}
	if ldapBindDN, err = config.GetString("auth:ldap:binddn"); err != nil {
		ldapBindDN = ""
	}
	if ldapBindPassword, err = config.GetString("auth:ldap:bindpassword"); err != nil {
		ldapBindPassword = ""
	}
	if ldapUserFilter, err = config.GetString("auth:ldap:userfilter"); err != nil {
		ldapUserFilter = "(email=%s)"
	}
	if ldapGroupFilter, err = config.GetString("auth:ldap:groupfilter"); err != nil {
		ldapGroupFilter = "(memberUid=%s)"
	}

	return nil
}

func hashPassword(u *auth.User) error {
	loadConfig()
	passwd, err := bcrypt.GenerateFromPassword([]byte(u.Password), cost)
	if err != nil {
		return err
	}
	u.Password = string(passwd)
	return nil
}

func token(data string, hash crypto.Hash) string {
	var tokenKey [keySize]byte
	n, err := rand.Read(tokenKey[:])
	for n < keySize || err != nil {
		n, err = rand.Read(tokenKey[:])
	}
	h := hash.New()
	h.Write([]byte(data))
	h.Write(tokenKey[:])
	h.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func newUserToken(u *auth.User) (*Token, error) {
	if u == nil {
		return nil, errors.New("User is nil")
	}
	if u.Email == "" {
		return nil, errors.New("Impossible to generate tokens for users without email")
	}
	if err := loadConfig(); err != nil {
		return nil, err
	}
	t := Token{}
	t.Creation = time.Now()
	t.Expires = tokenExpire
	t.Token = token(u.Email, crypto.SHA1)
	t.UserEmail = u.Email
	return &t, nil
}

func removeOldTokens(userEmail string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	var limit int
	if limit, err = config.GetInt("auth:max-simultaneous-sessions"); err != nil {
		return err
	}
	count, err := conn.Tokens().Find(bson.M{"useremail": userEmail}).Count()
	if err != nil {
		return err
	}
	diff := count - limit
	if diff < 1 {
		return nil
	}
	var tokens []map[string]interface{}
	err = conn.Tokens().Find(bson.M{"useremail": userEmail}).
		Select(bson.M{"_id": 1}).Sort("creation").Limit(diff).All(&tokens)
	if err != nil {
		return nil
	}
	ids := make([]interface{}, 0, len(tokens))
	for _, token := range tokens {
		ids = append(ids, token["_id"])
	}
	_, err = conn.Tokens().RemoveAll(bson.M{"_id": bson.M{"$in": ids}})
	return err
}

func checkPassword(passwordHash string, password string) error {
	if !validation.ValidateLength(password, passwordMinLen, passwordMaxLen) {
		return &tsuruErrors.ValidationError{Message: passwordError}
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) == nil {
		return nil
	}
	return auth.AuthenticationFailure{Message: "Authentication failed, wrong password."}
}

func createToken(u *auth.User, password string) (*Token, error) {
	if u.Email == "" {
		return nil, errors.New("User does not have an email")
	}
	if err := ldapBind(u.Email, password); err != nil {
		return nil, err
	}
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	token, err := newUserToken(u)
	if err != nil {
		return nil, err
	}
	err = conn.Tokens().Insert(token)
	go removeOldTokens(u.Email)
	return token, err
}

func getToken(header string) (*Token, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var t Token
	token, err := auth.ParseToken(header)
	if err != nil {
		return nil, err
	}
	err = conn.Tokens().Find(bson.M{"token": token}).One(&t)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, auth.ErrInvalidToken
		}
		return nil, err
	}
	if t.Expires > 0 && time.Until(t.Creation.Add(t.Expires)) < 1 {
		return nil, auth.ErrInvalidToken
	}
	return &t, nil
}

func deleteToken(token string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Tokens().Remove(bson.M{"token": token})
}

func deleteAllTokens(email string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Tokens().RemoveAll(bson.M{"useremail": email})
	return err
}

func createApplicationToken(appName string) (*Token, error) {
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	t := Token{
		Token:    token(appName, crypto.SHA1),
		Creation: time.Now(),
		Expires:  0,
		AppName:  appName,
	}
	err = conn.Tokens().Insert(t)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func ldapBind(uid, password string) error {
	err := loadConfig()
	if err != nil {
		panic(fmt.Sprintf("ERROR: %v", err.Error()))
	}
	client := &ldap.LDAPClient{
		Base:               ldapBaseDn,
		Host:               ldapHost,
		Port:               ldapPort,
		UseSSL:             ldapUseSSL,
		SkipTLS:            ldapSkipTLS,
		InsecureSkipVerify: ldapInsecureSkipVerify,
		ServerName:         ldapServerName,
		BindDN:             ldapBindDN,
		BindPassword:       ldapBindPassword,
		UserFilter:         ldapUserFilter,
		GroupFilter:        ldapGroupFilter,
		Attributes:         []string{"uidNumber", "cn", "email", "uid"},
	}

	// It is the responsibility of the caller to close the connection
	defer client.Close()

	ok, user, err := client.Authenticate(uid, password)
	if err != nil {
		return err
	}
	if !ok {
		return err
	}
	log.Printf("Authenticated user: %+v", user)

	groups, err := client.GetGroupsOfUser(user["uid"])
	if err != nil {
		return err
	}
	log.Printf("Authenticated user groups: %+v", groups)
	return err
}
