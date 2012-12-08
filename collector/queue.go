// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/globocom/config"
	"github.com/globocom/tsuru/app"
	"github.com/globocom/tsuru/log"
	"github.com/globocom/tsuru/queue"
	"sync/atomic"
	"time"
)

const MaxVisits = 50

type MessageHandler struct {
	closed int32
	server *queue.Server
}

func (h *MessageHandler) start() error {
	addr, err := config.GetString("queue-server")
	if err != nil {
		return err
	}
	h.server, err = queue.StartServer(addr)
	if err != nil {
		return fmt.Errorf("Could not start queue server at %s: %s", addr, err)
	}
	go h.handleMessages()
	return nil
}

func (h *MessageHandler) handleMessages() {
	for {
		if message, err := h.server.Message(-1); err == nil {
			go h.handle(message)
		} else if atomic.LoadInt32(&h.closed) == 0 {
			log.Printf("Failed to receive message: %s. Trying again...", err)
			continue
		} else {
			log.Printf("Connection closed, stop handling messages.")
			return
		}
	}
}

func (h *MessageHandler) handle(msg queue.Message) {
	if msg.Visits >= MaxVisits {
		log.Printf("Error handling %q: this message has been visited more than %d times.", msg.Action, MaxVisits)
		return
	}
	switch msg.Action {
	case app.RegenerateApprc:
		if len(msg.Args) < 1 {
			log.Printf("Error handling %q: this action requires at least 1 argument.", msg.Action)
			return
		}
		a := app.App{Name: msg.Args[0]}
		err := a.Get()
		if err != nil {
			log.Printf("Error handling %q: app %q does not exist.", msg.Action, a.Name)
			return
		}
		if a.State != "started" {
			format := "Error handling %q for the app %q:"
			switch a.State {
			case "error":
				format += " the app is in %q state."
			case "down":
				format += " the app is %s."
			default:
				format += ` The status of the app should be "started", but it is %q.`
				time.Sleep(time.Duration(msg.Visits+1) * time.Second)
				h.server.PutBack(msg)
			}
			log.Printf(format, msg.Action, a.Name, a.State)
			return
		}
		a.SerializeEnvVars()
	default:
		log.Printf("Error handling %q: invalid action.", msg.Action)
	}
}

func (h *MessageHandler) stop() error {
	atomic.StoreInt32(&h.closed, 1)
	return h.server.Close()
}
