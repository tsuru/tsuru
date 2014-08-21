// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"net"
	"sync"

	"github.com/bradfitz/go-smtpd/smtpd"
)

// Mail represents an email message that has been sent to the fake server.
type Mail struct {
	From string
	To   []string
	Data []byte
}

// SMTPServer is a fake SMTP server implementation.
//
// This SMTP server does not support authentication and is supposed to be used
// in tests only.
//
// Use NewSMTPServer create a new instance and start serving; SMTPServer.Addr()
// will get you the address of the server and Stop will stop the server,
// closing the listener. Every message that arrive at the server is stored in
// the MailBox slice.
type SMTPServer struct {
	// MailBox is the slice that stores all messages that has arrived to
	// the server while it's listening. Use the mutex to access it, or bad
	// things can happen.
	MailBox []Mail
	sync.RWMutex
	listener net.Listener
}

// NewSMTPServer creates a new SMTP server, for testing purposes.
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

// Addr returns the address of the server, in the form <host>:<port>.
func (s *SMTPServer) Addr() string {
	return s.listener.Addr().String()
}

// Stop stops the server. It's safe to call this method multiple times.
func (s *SMTPServer) Stop() {
	s.listener.Close()
}

// Reset resets the server, cleaning up the mailbox.
func (s *SMTPServer) Reset() {
	s.Lock()
	s.MailBox = nil
	s.Unlock()
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
	e.s.Lock()
	e.s.MailBox = append(e.s.MailBox, e.m)
	e.s.Unlock()
	return nil
}
