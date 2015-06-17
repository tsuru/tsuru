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

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
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

func addLogs(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if t.GetAppName() != app.InternalAppName {
		return &errors.HTTP{Code: http.StatusForbidden, Message: "this token is not allowed to execute this action"}
	}
	if r.Body == nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "body is required"}
	}
	defer r.Body.Close()
	logCh, errCh := app.LogReceiver()
	scanner := bufio.NewScanner(r.Body)
	for scanner.Scan() {
		var entry app.Applog
		data := bytes.TrimSpace(scanner.Bytes())
		if len(data) == 0 {
			continue
		}
		err := json.Unmarshal(data, &entry)
		if err != nil {
			close(logCh)
			return fmt.Errorf("error parsing log line %q: %s", string(data), err)
		}
		select {
		case logCh <- &entry:
		case err := <-errCh:
			close(logCh)
			return fmt.Errorf("error storing log: %s", err)
		}
	}
	close(logCh)
	err := scanner.Err()
	if err != nil {
		return fmt.Errorf("error waiting for log data: %s", err)
	}
	err = <-errCh
	if err != nil {
		return fmt.Errorf("error storing log: %s", err)
	}
	w.WriteHeader(http.StatusOK)
	return nil
}
