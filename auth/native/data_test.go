// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"bytes"
	"strings"

	"github.com/tsuru/config"
	check "gopkg.in/check.v1"
)

const resetDefaultContent = "Someone, hopefully you, requested to reset your password on tsuru"
const resetTemplateContent = "It is an email template to reset password"
const successDefaultContent = "Greetings!"
const successTemplateContent = "Congrats!"

func (s *S) TestResetWithoutTemplate(c *check.C) {
	var email bytes.Buffer
	tmp, err := getEmailResetPasswordTemplate()
	c.Assert(err, check.IsNil)
	err = tmp.Execute(&email, nil)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(email.String(), resetDefaultContent), check.Equals, true)
}

func (s *S) TestResetWithTemplate(c *check.C) {
	config.Set("reset-password-template", "testdata/email-reset-password.txt")
	defer config.Unset("reset-password-template")
	var email bytes.Buffer
	tmp, err := getEmailResetPasswordTemplate()
	c.Assert(err, check.IsNil)
	err = tmp.Execute(&email, nil)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(email.String(), resetTemplateContent), check.Equals, true)
}

func (s *S) TestResetWithNotFoundTemplate(c *check.C) {
	config.Set("reset-password-template", "testdata/wrong-email-reset-password.txt")
	defer config.Unset("reset-password-template")
	tmp, err := getEmailResetPasswordTemplate()
	c.Assert(tmp, check.IsNil)
	c.Assert(err, check.NotNil)
}

func (s *S) TestResetSuccessWithoutTemplate(c *check.C) {
	var email bytes.Buffer
	tmp, err := getEmailResetPasswordSucessfullyTemplate()
	c.Assert(err, check.IsNil)
	err = tmp.Execute(&email, nil)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(email.String(), successDefaultContent), check.Equals, true)
}

func (s *S) TestResetSuccessWithTemplate(c *check.C) {
	config.Set("reset-password-successfully-template", "testdata/email-reset-success.txt")
	defer config.Unset("reset-password-successfully-template")
	var email bytes.Buffer
	tmp, err := getEmailResetPasswordSucessfullyTemplate()
	c.Assert(err, check.IsNil)
	err = tmp.Execute(&email, nil)
	c.Assert(err, check.IsNil)
	c.Assert(strings.Contains(email.String(), successTemplateContent), check.Equals, true)
}

func (s *S) TestResetSuccessWithWrongTemplate(c *check.C) {
	config.Set("reset-password-successfully-template", "testdata/wrong-email-reset-success.txt")
	defer config.Unset("reset-password-successfully-template")
	tmp, err := getEmailResetPasswordSucessfullyTemplate()
	c.Assert(tmp, check.IsNil)
	c.Assert(err, check.NotNil)
}
