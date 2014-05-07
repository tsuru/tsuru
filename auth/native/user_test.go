package native

import (
	"errors"
	"github.com/tsuru/config"
	"launchpad.net/gocheck"
	"runtime"
	"sync"
)

func (s *S) TestSendEmail(c *gocheck.C) {
	defer s.server.Reset()
	err := sendEmail("something@tsuru.io", []byte("Hello world!"))
	c.Assert(err, gocheck.IsNil)
	s.server.Lock()
	defer s.server.Unlock()
	m := s.server.MailBox[0]
	c.Assert(m.To, gocheck.DeepEquals, []string{"something@tsuru.io"})
	c.Assert(m.From, gocheck.Equals, "root")
	c.Assert(m.Data, gocheck.DeepEquals, []byte("Hello world!\r\n"))
}

func (s *S) TestSendEmailUndefinedSMTPServer(c *gocheck.C) {
	old, _ := config.Get("smtp:server")
	defer config.Set("smtp:server", old)
	config.Unset("smtp:server")
	err := sendEmail("something@tsuru.io", []byte("Hello world!"))
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Setting "smtp:server" is not defined`)
}

func (s *S) TestSendEmailUndefinedUser(c *gocheck.C) {
	old, _ := config.Get("smtp:user")
	defer config.Set("smtp:user", old)
	config.Unset("smtp:user")
	err := sendEmail("something@tsuru.io", []byte("Hello world!"))
	c.Assert(err, gocheck.NotNil)
	c.Assert(err.Error(), gocheck.Equals, `Setting "smtp:user" is not defined`)
}

func (s *S) TestSendEmailUndefinedSMTPPassword(c *gocheck.C) {
	defer s.server.Reset()
	old, _ := config.Get("smtp:password")
	defer config.Set("smtp:password", old)
	config.Unset("smtp:password")
	err := sendEmail("something@tsuru.io", []byte("Hello world!"))
	c.Assert(err, gocheck.IsNil)
	s.server.Lock()
	defer s.server.Unlock()
	m := s.server.MailBox[0]
	c.Assert(m.To, gocheck.DeepEquals, []string{"something@tsuru.io"})
	c.Assert(m.From, gocheck.Equals, "root")
	c.Assert(m.Data, gocheck.DeepEquals, []byte("Hello world!\r\n"))
}

func (s *S) TestSMTPServer(c *gocheck.C) {
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
		c.Check(err, gocheck.DeepEquals, t.failure)
		c.Check(server, gocheck.Equals, t.output)
	}
}

func (s *S) TestGeneratePassword(c *gocheck.C) {
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
		c.Check(p, gocheck.Not(gocheck.Equals), first)
	}
}
