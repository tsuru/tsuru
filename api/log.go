// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/log"
	"golang.org/x/net/websocket"
)

func addLogs(ws *websocket.Conn) {
	var err error
	defer func() {
		data := map[string]interface{}{}
		if err != nil {
			data["error"] = err.Error()
			log.Error(err.Error())
		} else {
			data["error"] = nil
		}
		msg, _ := json.Marshal(data)
		ws.Write(msg)
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
	dispatcher := app.NewlogDispatcher()
	scanner := bufio.NewScanner(stream)
	var entry app.Applog
	for scanner.Scan() {
		data := bytes.TrimSpace(scanner.Bytes())
		if len(data) == 0 {
			continue
		}
		err := json.Unmarshal(data, &entry)
		if err != nil {
			dispatcher.Stop()
			return fmt.Errorf("wslogs: parsing log line %q: %s", string(data), err)
		}
		err = dispatcher.Send(&entry)
		if err != nil {
			// Do not disconnect by returning here, dispatcher will already
			// retry db connection and we gain nothing by ending the WS
			// connection.
			log.Errorf("wslogs: error storing log: %s", err)
		}
	}
	err := dispatcher.Stop()
	if err != nil {
		return fmt.Errorf("wslogs: error storing log: %s", err)
	}
	err = scanner.Err()
	if err != nil {
		return fmt.Errorf("wslogs: waiting for log data: %s", err)
	}
	return nil
}
