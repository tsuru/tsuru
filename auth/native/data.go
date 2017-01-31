// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package native

import (
	"text/template"

	"github.com/tsuru/config"
)

var passwordChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz1234567890_@#$%^&*()~[]{}?=-+,.<>:;`"

func getEmailResetPasswordTemplate() (*template.Template, error) {
	templateFile, _ := config.GetString("reset-password-template")
	if templateFile != "" {
		return template.ParseFiles(templateFile)
	}
	return template.Must(template.New("reset").Parse(`Subject: [tsuru] Password reset process
To: {{.UserEmail}}

Someone, hopefully you, requested to reset your password on tsuru. You will
need to use the following token to finish this process:

{{.Token}}

If you think this is email is wrong, just ignore it.`)), nil
}

func getEmailResetPasswordSucessfullyTemplate() (*template.Template, error) {
	templateFile, _ := config.GetString("reset-password-successfully-template")
	if templateFile != "" {
		return template.ParseFiles(templateFile)
	}
	return template.Must(template.New("reset").Parse(`Subject: [tsuru] Password successfully reset
To: {{.email}}

Greetings!

This message is the confirmation that your password has been reset. The new password is:

{{.password}}

Use it to authenticate with tsuru server, and change it later.`)), nil
}
