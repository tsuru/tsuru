// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package log

func NewSyslogLogger(tag string, debug bool) Logger {
	panic("syslog doesn't work on Windows")
}
