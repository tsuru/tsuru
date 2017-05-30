// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"io"
	"sync"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/log"
	"golang.org/x/net/websocket"
)

var (
	onceDispatcher   = sync.Once{}
	globalDispatcher *app.LogDispatcher
)

func getDispatcher() *app.LogDispatcher {
	onceDispatcher.Do(func() {
		queueSize, _ := config.GetInt("server:app-log-buffer-size")
		if queueSize == 0 {
			queueSize = 500000
		}
		globalDispatcher = app.NewlogDispatcher(queueSize)
	})
	return globalDispatcher
}

func addLogs(ws *websocket.Conn) {
	var err error
	defer func() {
		msg := &errMsg{}
		if err != nil {
			msg.Error = err.Error()
			log.Errorf("failure in logs webservice: %s", err)
		}
		websocket.JSON.Send(ws, msg)
		ws.Close()
	}()
	req := ws.Request()
	t := context.GetAuthToken(req)
	if t == nil {
		err = errors.Errorf("wslogs: no token")
		return
	}
	if t.GetAppName() != app.InternalAppName {
		err = errors.Errorf("wslogs: invalid token app name: %q", t.GetAppName())
		return
	}
	err = scanLogs(ws)
	if err != nil {
		return
	}
}

func scanLogs(stream io.Reader) error {
	dispatcher := getDispatcher()
	decoder := json.NewDecoder(stream)
	for {
		var entry app.Applog
		err := decoder.Decode(&entry)
		if err != nil {
			if err == io.EOF {
				break
			}
			return errors.Wrap(err, "wslogs: parsing log line")
		}
		if entry.Date.IsZero() || entry.AppName == "" || entry.Message == "" {
			continue
		}
		err = dispatcher.Send(&entry)
		if err != nil {
			return err
		}
	}
	return nil
}

type errMsg struct {
	Error string `json:"error"`
}
