// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package queue_test

import (
	"github.com/globocom/tsuru/queue"
	"log"
)

// This example demonstrates how to start a new Server.
func ExampleStartServer() {
	server, err := queue.StartServer("127.0.0.1:0")
	if err != nil {
		log.Panicf("Failed to start the server: %s", err)
	}
	defer server.Close()
}

// This example demonstrates how to dial to a queue server.
func ExampleDial() {
	messages, errors, err := queue.Dial("127.0.0.1:2021")
	if err != nil {
		log.Panicf("Failed to dial: %s", err)
	}
	message := queue.Message{
		Action: "delete-files",
		Args:   []string{"/home/gopher/file.txt"},
	}
	messages <- message
	close(messages) // closes the channel and the connection.
	if err, ok := <-errors; ok && err != nil {
		log.Panicf("Failed to send the message: %s", err)
	}
}
