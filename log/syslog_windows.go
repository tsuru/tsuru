// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

import "github.com/pkg/errors"

func NewSyslogLogger(tag string, debug bool) (Logger, error) {
	return nil, errors.New("syslog doesn't work on Windows")
}
