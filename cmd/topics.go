// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

const targetTopic = `Target is used to manage the address of the remote tsuru server.

Each target is identified by a label and a HTTP/HTTPS address. The client
requires at least one target to connect to, there's no default target. A user
may have multiple targets, but only one will be used at a time.`
