package native

import (
	"bytes"
	"strings"

	"gopkg.in/check.v1"

	"github.com/tsuru/config"
)

const resetDefaultContent = "Someone, hopefully you, requested to reset your password on tsuru"
const resetTemplateContent = "It is an email template to reset password"
const successDefaultContent = "Greetings!"
const successTemplateContent = "Congrats!"

func (s *S) TestResetWithoutTemplate(c *check.C) {
	var email bytes.Buffer
	tmp := getEmailResetPasswordTemplate()
	err := tmp.Execute(&email, nil)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(email.String(), resetDefaultContent), check.Equals, true)
}

func (s *S) TestResetWithTemplate(c *check.C) {
	config.Set("reset-password-template", "testdata/email-reset-password.txt")
	defer config.Unset("reset-password-template")
	var email bytes.Buffer
	tmp := getEmailResetPasswordTemplate()
	err := tmp.Execute(&email, nil)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(email.String(), resetTemplateContent), check.Equals, true)
}

func (s *S) TestResetWithNotFoundTemplate(c *check.C) {
	config.Set("reset-password-template", "testdata/wrong-email-reset-password.txt")
	defer config.Unset("reset-password-template")
	var email bytes.Buffer
	tmp := getEmailResetPasswordTemplate()
	err := tmp.Execute(&email, nil)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(email.String(), resetDefaultContent), check.Equals, true)
}

func (s *S) TestResetSuccessWithoutTemplate(c *check.C) {
	var email bytes.Buffer
	tmp := getEmailResetPasswordSucessfullyTemplate()
	err := tmp.Execute(&email, nil)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(email.String(), successDefaultContent), check.Equals, true)
}

func (s *S) TestResetSuccessWithTemplate(c *check.C) {
	config.Set("reset-password-successfully-template", "testdata/email-reset-success.txt")
	defer config.Unset("reset-password-successfully-template")
	var email bytes.Buffer
	tmp := getEmailResetPasswordSucessfullyTemplate()
	err := tmp.Execute(&email, nil)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(email.String(), successTemplateContent), check.Equals, true)
}

func (s *S) TestResetSuccessWithWrongTemplate(c *check.C) {
	config.Set("reset-password-successfully-template", "testdata/wrong-email-reset-success.txt")
	defer config.Unset("reset-password-successfully-template")
	var email bytes.Buffer
	tmp := getEmailResetPasswordSucessfullyTemplate()
	err := tmp.Execute(&email, nil)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(email.String(), successDefaultContent), check.Equals, true)
}
