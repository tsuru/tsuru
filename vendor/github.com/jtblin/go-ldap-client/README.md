# go-ldap-client

Simple ldap client to authenticate, retrieve basic information and groups for a user.

# Usage

[Go Doc](https://godoc.org/github.com/jtblin/go-ldap-client)

See [example](example_test.go). The only external dependency is [gopkg.in/ldap.v2](http://gopkg.in/ldap.v2).

```golang
package main

import (
	"log"

	"github.com/jtblin/go-ldap-client"
)

func main() {
	client := &ldap.LDAPClient{
		Base:         "dc=example,dc=com",
		Host:         "ldap.example.com",
		Port:         389,
		UseSSL:       false,
		BindDN:       "uid=readonlysuer,ou=People,dc=example,dc=com",
		BindPassword: "readonlypassword",
		UserFilter:   "(uid=%s)",
		GroupFilter: "(memberUid=%s)",
		Attributes:   []string{"givenName", "sn", "mail", "uid"},
	}
	// It is the responsibility of the caller to close the connection
	defer client.Close()

	ok, user, err := client.Authenticate("username", "password")
	if err != nil {
		log.Fatalf("Error authenticating user %s: %+v", "username", err)
	}
	if !ok {
		log.Fatalf("Authenticating failed for user %s", "username")
	}
	log.Printf("User: %+v", user)
	
	groups, err := client.GetGroupsOfUser("username")
	if err != nil {
		log.Fatalf("Error getting groups for user %s: %+v", "username", err)
	}
	log.Printf("Groups: %+v", groups) 
}
```

## SSL (ldaps)

If you use SSL, you will need to pass the server name for certificate verification
or skip domain name verification e.g.`client.ServerName = "ldap.example.com"`.

# Why?

There are already [tons](https://godoc.org/?q=ldap) of ldap libraries for `golang` but most of them
are just forks of another one, most of them are too low level or too limited (e.g. do not return errors 
which make it hard to troubleshoot issues).