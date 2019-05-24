// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"io"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/log"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
	"golang.org/x/net/websocket"
)

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
	decoder := json.NewDecoder(stream)
	for {
		var entry appTypes.Applog
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
		err = servicemanager.AppLog.Enqueue(&entry)
		if err != nil {
			return err
		}
	}
	return nil
}

type errMsg struct {
	Error string `json:"error"`
}
