// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package authtest

import (
	"net"
	"net/smtp"
	"strings"
	"testing"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type S struct{}

var _ = check.Suite(&S{})

func (s *S) TestFakeMailAddress(c *check.C) {
	fake := fakeMailAddress("gopher@tsuru.io")
	c.Assert(fake.Email(), check.Equals, "gopher@tsuru.io")
	c.Assert(fake.Hostname(), check.Equals, "tsuru.io")
	fake = fakeMailAddress("gopher")
	c.Assert(fake.Email(), check.Equals, "gopher")
	c.Assert(fake.Hostname(), check.Equals, "")
}

func (s *S) TestFakeEnvelope(c *check.C) {
	server := SMTPServer{}
	fake := newFakeEnvelope("tsuru@globo.com", &server)
	err := fake.BeginData()
	c.Assert(err, check.NotNil)
	fake.AddRecipient(fakeMailAddress("gandalf@globo.com"))
	err = fake.BeginData()
	c.Assert(err, check.IsNil)
	fake.Write([]byte("Hello world!"))
	fake.Write([]byte("Hello again!"))
	fake.Close()
	c.Assert(server.MailBox, check.HasLen, 1)
	c.Assert(server.MailBox[0], check.DeepEquals, fake.m)
}

func (s *S) TestSMTPServerStart(c *check.C) {
	server, err := NewSMTPServer()
	c.Assert(err, check.IsNil)
	conn, err := net.Dial("tcp", server.Addr())
	c.Assert(err, check.IsNil)
	conn.Close()
}

func (s *S) TestSMTPServerStop(c *check.C) {
	server, err := NewSMTPServer()
	c.Assert(err, check.IsNil)
	server.Stop()
	_, err = net.Dial("tcp", server.Addr())
	c.Assert(err, check.NotNil)
}

func (s *S) TestSMTPServerRecordsMailMessages(c *check.C) {
	server, err := NewSMTPServer()
	c.Assert(err, check.IsNil)
	defer server.Stop()
	to := []string{"gopher1@tsuru.io", "gopher2@tsuru.io"}
	err = smtp.SendMail(server.Addr(), nil, "gopher@tsuru.io", to, []byte("Hello world!"))
	c.Assert(err, check.IsNil)
	c.Assert(server.MailBox, check.HasLen, 1)
	want := Mail{
		From: "gopher@tsuru.io",
		To:   to,
		Data: []byte("Hello world!\r\n"),
	}
	c.Assert(server.MailBox[0], check.DeepEquals, want)
}

func (s *S) TestSMTPServerReset(c *check.C) {
	server, err := NewSMTPServer()
	c.Assert(err, check.IsNil)
	defer server.Stop()
	to := []string{"gopher1@tsuru.io", "gopher2@tsuru.io"}
	err = smtp.SendMail(server.Addr(), nil, "gopher@tsuru.io", to, []byte("Hello world!"))
	c.Assert(err, check.IsNil)
	c.Assert(server.MailBox, check.HasLen, 1)
	server.Reset()
	c.Assert(server.MailBox, check.HasLen, 0)
}

type fakeMailAddress string

func (m fakeMailAddress) Email() string {
	return string(m)
}

func (m fakeMailAddress) Hostname() string {
	s := string(m)
	if p := strings.Index(s, "@"); p > -1 {
		return s[p+1:]
	}
	return ""
}
