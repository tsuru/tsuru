// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/log"
	"golang.org/x/net/websocket"
)

func logRemove(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	appName := r.URL.Query().Get("app")
	if appName != "" {
		u, err := t.User()
		if err != nil {
			return err
		}
		a, err := getApp(r.URL.Query().Get("app"), u, r)
		if err != nil {
			return err
		}
		return app.LogRemove(&a)
	}
	return app.LogRemove(nil)
}

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
	logCh, errCh := app.LogReceiver()
	scanner := bufio.NewScanner(ws)
	for scanner.Scan() {
		var entry app.Applog
		data := bytes.TrimSpace(scanner.Bytes())
		if len(data) == 0 {
			continue
		}
		err := json.Unmarshal(data, &entry)
		if err != nil {
			close(logCh)
			err = fmt.Errorf("wslogs: parsing log line %q: %s", string(data), err)
			return
		}
		select {
		case logCh <- &entry:
		case err := <-errCh:
			close(logCh)
			err = fmt.Errorf("wslogs: storing log: %s", err)
			return
		}
	}
	close(logCh)
	err = scanner.Err()
	if err != nil {
		err = fmt.Errorf("wslogs: waiting for log data: %s", err)
		return
	}
	err = <-errCh
	if err != nil {
		err = fmt.Errorf("wslogs: storing log: %s", err)
	}
}
