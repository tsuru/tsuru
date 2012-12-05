// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package queue implements a queue based on channels and networking.
//
// It is based on concepts from old/netchan and a lot of discussion about this
// theme on the internet. The implementation present here is specific to tsuru,
// but could be more generic.
//
// This package provides two basic functions: StartServer and Dial. StartServer
// should be used in the server side, while Dial is designed to be used in the
// client side.
//
// Here is a example of using StartServer:
//
//     server, err := queue.StartServer("127.0.0.1:0")
//     if err != nil {
//         panic(err)
//     }
//     // Gets a message from the client, or times out after 5 seconds.
//     message, err := server.Message(5e9)
//     if err != nil {
//         panic(err)
//     }
//     // do something with the message
//
// Dial is used to connect to the server. The communication between the server
// and the client happens through channels:
//
//     messages, errors, err := Dial("10.10.10.10:9058")
//     if err != nil {
//         panic(err)
//     }
//     messages <- Message{Action: "regenerate apprc", Args: []string{"g1"}}
//
// It's up to the server and the client decide the meaning of a message.
package queue
