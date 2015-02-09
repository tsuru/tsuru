package native

import (
	"errors"
	"runtime"
	"sync"

	"github.com/tsuru/config"
	"gopkg.in/check.v1"
)

func (s *S) TestSendEmail(c *check.C) {
	defer s.server.Reset()
	err := sendEmail("something@tsuru.io", []byte("Hello world!"))
	c.Assert(err, check.IsNil)
	s.server.Lock()
	defer s.server.Unlock()
	m := s.server.MailBox[0]
	c.Assert(m.To, check.DeepEquals, []string{"something@tsuru.io"})
	c.Assert(m.From, check.Equals, "root")
	c.Assert(m.Data, check.DeepEquals, []byte("Hello world!\r\n"))
}

func (s *S) TestSendEmailUndefinedSMTPServer(c *check.C) {
	old, _ := config.Get("smtp:server")
	defer config.Set("smtp:server", old)
	config.Unset("smtp:server")
	err := sendEmail("something@tsuru.io", []byte("Hello world!"))
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, `Setting "smtp:server" is not defined`)
}

func (s *S) TestSendEmailUndefinedUser(c *check.C) {
	old, _ := config.Get("smtp:user")
	defer config.Set("smtp:user", old)
	config.Unset("smtp:user")
	err := sendEmail("something@tsuru.io", []byte("Hello world!"))
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Equals, `Setting "smtp:user" is not defined`)
}

func (s *S) TestSendEmailUndefinedSMTPPassword(c *check.C) {
	defer s.server.Reset()
	old, _ := config.Get("smtp:password")
	defer config.Set("smtp:password", old)
	config.Unset("smtp:password")
	err := sendEmail("something@tsuru.io", []byte("Hello world!"))
	c.Assert(err, check.IsNil)
	s.server.Lock()
	defer s.server.Unlock()
	m := s.server.MailBox[0]
	c.Assert(m.To, check.DeepEquals, []string{"something@tsuru.io"})
	c.Assert(m.From, check.Equals, "root")
	c.Assert(m.Data, check.DeepEquals, []byte("Hello world!\r\n"))
}

func (s *S) TestSMTPServer(c *check.C) {
	var tests = []struct {
		input   string
		output  string
		failure error
	}{
		{"smtp.gmail.com", "smtp.gmail.com:25", nil},
		{"smtp.gmail.com:465", "smtp.gmail.com:465", nil},
		{"", "", errors.New(`Setting "smtp:server" is not defined`)},
	}
	old, _ := config.Get("smtp:server")
	defer config.Set("smtp:server", old)
	for _, t := range tests {
		config.Set("smtp:server", t.input)
		server, err := smtpServer()
		c.Check(err, check.DeepEquals, t.failure)
		c.Check(server, check.Equals, t.output)
	}
}

func (s *S) TestGeneratePassword(c *check.C) {
	go runtime.GOMAXPROCS(runtime.GOMAXPROCS(4))
	passwords := make([]string, 1000)
	var wg sync.WaitGroup
	for i := range passwords {
		wg.Add(1)
		go func(i int) {
			passwords[i] = generatePassword(8)
			wg.Done()
		}(i)
	}
	wg.Wait()
	first := passwords[0]
	for _, p := range passwords[1:] {
		c.Check(p, check.Not(check.Equals), first)
	}
}
