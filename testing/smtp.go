// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"github.com/bradfitz/go-smtpd/smtpd"
	"net"
	"strings"
	"sync"
)

type Mail struct {
	From string
	To   []string
	Data []byte
}

type SMTPServer struct {
	MailBox  []Mail
	mutex    sync.Mutex
	listener net.Listener
}

func NewSMTPServer() (*SMTPServer, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := SMTPServer{listener: l}
	server := smtpd.Server{
		Addr:      s.Addr(),
		Hostname:  "localhost",
		PlainAuth: false,
		OnNewMail: func(c smtpd.Connection, from smtpd.MailAddress) (smtpd.Envelope, error) {
			return newFakeEnvelope(from.Email(), &s), nil
		},
	}
	go server.Serve(s.listener)
	return &s, nil
}

func (s *SMTPServer) Addr() string {
	return s.listener.Addr().String()
}

func (s *SMTPServer) Stop() {
	s.listener.Close()
}

type fakeEnvelope struct {
	s *SMTPServer
	m Mail
}

func newFakeEnvelope(from string, s *SMTPServer) *fakeEnvelope {
	e := fakeEnvelope{m: Mail{From: from}, s: s}
	return &e
}

func (e *fakeEnvelope) AddRecipient(rcpt smtpd.MailAddress) error {
	e.m.To = append(e.m.To, rcpt.Email())
	return nil
}

func (e *fakeEnvelope) BeginData() error {
	if len(e.m.To) == 0 {
		return smtpd.SMTPError("554 5.5.1 Error: no valid recipients")
	}
	return nil
}

func (e *fakeEnvelope) Write(line []byte) error {
	e.m.Data = append(e.m.Data, line...)
	return nil
}

func (e *fakeEnvelope) Close() error {
	e.s.mutex.Lock()
	e.s.MailBox = append(e.s.MailBox, e.m)
	e.s.mutex.Unlock()
	return nil
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
