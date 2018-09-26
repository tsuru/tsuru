package godap

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// test a very simple LDAP server with hard coded bind and search results
func TestLdapServer1(t *testing.T) {

	hs := make([]LDAPRequestHandler, 0)

	// use a LDAPBindFuncHandler to provide a callback function to respond
	// to bind requests
	hs = append(hs, &LDAPBindFuncHandler{LDAPBindFunc: func(binddn string, bindpw []byte) bool {
		if strings.HasPrefix(binddn, "cn=Joe Dimaggio,") && string(bindpw) == "marylinisthebomb" {
			return true
		}
		return false
	}})

	// use a LDAPSimpleSearchFuncHandler to reply to search queries
	hs = append(hs, &LDAPSimpleSearchFuncHandler{LDAPSimpleSearchFunc: func(req *LDAPSimpleSearchRequest) []*LDAPSimpleSearchResultEntry {

		ret := make([]*LDAPSimpleSearchResultEntry, 0, 1)

		// here we produce a single search result that matches whatever
		// they are searching for
		if req.FilterAttr == "uid" {
			ret = append(ret, &LDAPSimpleSearchResultEntry{
				DN: "cn=" + req.FilterValue + "," + req.BaseDN,
				Attrs: map[string]interface{}{
					"sn":            req.FilterValue,
					"cn":            req.FilterValue,
					"uid":           req.FilterValue,
					"homeDirectory": "/home/" + req.FilterValue,
					"objectClass": []string{
						"top",
						"posixAccount",
						"inetOrgPerson",
					},
				},
			})
		}

		return ret

	}})

	s := &LDAPServer{
		Handlers: hs,
	}

	go s.ListenAndServe("127.0.0.1:10000")

	// yeah, you gotta have ldapsearch (from openldap) installed; but if you're
	// serious about hurting yourself with ldap, you've already done this
	b, err := exec.Command("/usr/bin/ldapsearch",
		`-H`,
		`ldap://127.0.0.1:10000/`,
		`-Dcn=Joe Dimaggio,dc=example,dc=net`,
		`-wmarylinisthebomb`,
		`-v`,
		`-bou=people,dc=example,dc=net`,
		`(uid=jfk)`,
	).CombinedOutput()
	fmt.Printf("RESULT1: %s\n", string(b))
	if err != nil {
		t.Fatalf("Error executing: %v", err)
	}

	bstr := string(b)

	if !strings.Contains(bstr, "dn: cn=jfk,ou=people,dc=example,dc=net") {
		t.Fatalf("Didn't find expected result string")
	}
	if !strings.Contains(bstr, "numEntries: 1") {
		t.Fatalf("Should have found exactly one result")
	}

}
