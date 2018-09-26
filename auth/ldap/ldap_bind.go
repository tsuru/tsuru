package ldap

import (
	"fmt"
	"log"

	"github.com/jtblin/go-ldap-client"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"

	ldapv2 "gopkg.in/ldap.v2"
)

var (
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
	ldapGroupMustBePresent string
)

func loadConfig() error {
	var err error
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
	if ldapGroupMustBePresent, err = config.GetString("auth:ldap:groupmustbepresent"); err != nil {
		ldapGroupMustBePresent = ""
	}

	return nil
}

func createToken(u *auth.User, password string) (*native.Token, error) {
	if u.Email == "" {
		return nil, errors.New("User does not have an email")
	}
	if err := ldapBind(u.Email, password); err != nil {
		ldapError, isLdapError := err.(*ldapv2.Error)
		if isLdapError {
			if ldapError.ResultCode == 49 {
				return nil, auth.AuthenticationFailure{Message: "Authentication failed, wrong password."}
			}
		}
		return nil, err
	}
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	token, err := native.NewUserToken(u)
	if err != nil {
		return nil, err
	}
	err = conn.Tokens().Insert(token)
	go native.RemoveOldTokens(u.Email)
	return token, err
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
	log.Printf("LDAP Authenticated user: %+v", user)

	groups, err := client.GetGroupsOfUser(user["uid"])
	if err != nil {
		return err
	}
	log.Printf("LDAP Authenticated user groups: %+v", groups)
	if ldapGroupMustBePresent != "" {
		err = errors.Errorf("LDAP user %s not present member on group %s.", user["uid"], ldapGroupMustBePresent)
		for _, group := range groups {
			if group == ldapGroupMustBePresent {
				return nil
			}
		}
	}
	return err
}
