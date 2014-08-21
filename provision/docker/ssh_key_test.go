// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"crypto/rsa"
	"strings"

	"launchpad.net/gocheck"
)

var _ = gocheck.Suite(&SSHKeySuite{})

type SSHKeySuite struct {
	keyPair *rsa.PrivateKey
}

func (s *SSHKeySuite) SetUpSuite(c *gocheck.C) {
	var err error
	buf := bytes.NewBufferString(strings.Repeat("something happened", 10240))
	s.keyPair, err = rsa.GenerateKey(buf, 512)
	c.Assert(err, gocheck.IsNil)
}

func (s *SSHKeySuite) TestMarshalKey(c *gocheck.C) {
	privKey, pubKey, err := marshalKey(s.keyPair)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(pubKey), gocheck.Equals, publicAuthorizedKey)
	c.Assert(string(privKey), gocheck.Equals, privateKeyPEM)
}

var privateKeyPEM = `-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJBANE0bCR0Xkg83QFZwwtmbzT6dPusOOnXEcre/zzHbfngVC5O2R9/
n6uKAb1eYapaA1HINSRZULUH0uYnTjq8P3MCAwEAAQJAdagf8diodcQVH39WHIE9
pfP9+tT/JTRZw1jq/0nB5jdpj0/XCEGurfloC8TY9ObN/Q4e3bqnosAvzz1iHoSi
wQIhAOVuZWRzb21ldGhpbmcgaGFwcGVuZWRzb21ldGhpbmcjAiEA6W5nIGhhcHBl
bmVkc29tZXRoaW5nIGhhcHBlbmVkc3ECIE0NaGv2AMQiwJeYYQWtcqDW3EiUbOTx
h8ibvB6c2gE1AiAo3k1r3RqCJwt7IoFNvIp4osLNAqlHgT7eAq+fflzx4QIhAMbw
OcKgmj2XOOBWUMeOCDOTBbpSdjt9JKIF1ImCoBG1
-----END RSA PRIVATE KEY-----
`

var publicAuthorizedKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAQQDRNGwkdF5IPN0BWcMLZm80+nT7rDjp1xHK3v88x2354FQuTtkff5+rigG9XmGqWgNRyDUkWVC1B9LmJ046vD9z\n"
