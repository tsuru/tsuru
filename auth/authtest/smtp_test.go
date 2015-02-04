// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package authtest

import (
	"net"
	"net/smtp"
	"strings"
	"testing"

	"launchpad.net/gocheck"
)

func Test(t *testing.T) {
	gocheck.TestingT(t)
}

type S struct{}

var _ = gocheck.Suite(&S{})

func (s *S) TestFakeMailAddress(c *gocheck.C) {
	fake := fakeMailAddress("gopher@tsuru.io")
	c.Assert(fake.Email(), gocheck.Equals, "gopher@tsuru.io")
	c.Assert(fake.Hostname(), gocheck.Equals, "tsuru.io")
	fake = fakeMailAddress("gopher")
	c.Assert(fake.Email(), gocheck.Equals, "gopher")
	c.Assert(fake.Hostname(), gocheck.Equals, "")
}

func (s *S) TestFakeEnvelope(c *gocheck.C) {
	server := SMTPServer{}
	fake := newFakeEnvelope("tsuru@globo.com", &server)
	err := fake.BeginData()
	c.Assert(err, gocheck.NotNil)
	fake.AddRecipient(fakeMailAddress("gandalf@globo.com"))
	err = fake.BeginData()
	c.Assert(err, gocheck.IsNil)
	fake.Write([]byte("Hello world!"))
	fake.Write([]byte("Hello again!"))
	fake.Close()
	c.Assert(server.MailBox, gocheck.HasLen, 1)
	c.Assert(server.MailBox[0], gocheck.DeepEquals, fake.m)
}

func (s *S) TestSMTPServerStart(c *gocheck.C) {
	server, err := NewSMTPServer()
	c.Assert(err, gocheck.IsNil)
	conn, err := net.Dial("tcp", server.Addr())
	c.Assert(err, gocheck.IsNil)
	conn.Close()
}

func (s *S) TestSMTPServerStop(c *gocheck.C) {
	server, err := NewSMTPServer()
	c.Assert(err, gocheck.IsNil)
	server.Stop()
	_, err = net.Dial("tcp", server.Addr())
	c.Assert(err, gocheck.NotNil)
}

func (s *S) TestSMTPServerRecordsMailMessages(c *gocheck.C) {
	server, err := NewSMTPServer()
	c.Assert(err, gocheck.IsNil)
	defer server.Stop()
	to := []string{"gopher1@tsuru.io", "gopher2@tsuru.io"}
	err = smtp.SendMail(server.Addr(), nil, "gopher@tsuru.io", to, []byte("Hello world!"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(server.MailBox, gocheck.HasLen, 1)
	want := Mail{
		From: "gopher@tsuru.io",
		To:   to,
		Data: []byte("Hello world!\r\n"),
	}
	c.Assert(server.MailBox[0], gocheck.DeepEquals, want)
}

func (s *S) TestSMTPServerReset(c *gocheck.C) {
	server, err := NewSMTPServer()
	c.Assert(err, gocheck.IsNil)
	defer server.Stop()
	to := []string{"gopher1@tsuru.io", "gopher2@tsuru.io"}
	err = smtp.SendMail(server.Addr(), nil, "gopher@tsuru.io", to, []byte("Hello world!"))
	c.Assert(err, gocheck.IsNil)
	c.Assert(server.MailBox, gocheck.HasLen, 1)
	server.Reset()
	c.Assert(server.MailBox, gocheck.HasLen, 0)
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
