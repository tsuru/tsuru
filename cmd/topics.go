// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

const targetTopic = `In tsuru, a target is the address of the remote tsuru server.

Each target is identified by a label and a HTTP/HTTPS address. The client
requires at least one target to connect to, there's no default target. A user
may have multiple targets, but he/she will be able to use only per session.

The following commands are used to manage targets in the client:

  * target-add: adds a new target to the list os available targets
  * target-list: list available targets, marking the current
  * target-remove: removes a target by its label
  * target-set: defines the current target, to which the CLI will send next
    commands

See each command usage by running %s help <commandname>
`
