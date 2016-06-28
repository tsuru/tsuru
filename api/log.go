// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/log"
	"golang.org/x/net/websocket"
)

func addLogs(ws *websocket.Conn) {
	var err error
	defer func() {
		msg := &errMsg{}
		if err != nil {
			msg.Error = err.Error()
			log.Error(err.Error())
		}
		websocket.JSON.Send(ws, msg)
		ws.Close()
	}()
	req := ws.Request()
	t := context.GetAuthToken(req)
	if t == nil {
		err = fmt.Errorf("wslogs: no token")
		return
	}
	if t.GetAppName() != app.InternalAppName {
		err = fmt.Errorf("wslogs: invalid token app name: %q", t.GetAppName())
		return
	}
	err = scanLogs(ws)
	if err != nil {
		return
	}
}

func scanLogs(stream io.Reader) error {
	queueSize, _ := config.GetInt("server:app-log-buffer-size")
	if queueSize == 0 {
		queueSize = 500000
	}
	dispatcher := app.NewlogDispatcher(queueSize, runtime.NumCPU())
	decoder := json.NewDecoder(stream)
	for {
		var entry app.Applog
		err := decoder.Decode(&entry)
		if err != nil {
			if err == io.EOF {
				break
			}
			dispatcher.Stop()
			return fmt.Errorf("wslogs: parsing log line: %s", err)
		}
		dispatcher.Send(&entry)
	}
	dispatcher.Stop()
	return nil
}

type errMsg struct {
	Error string `json:"error"`
}
