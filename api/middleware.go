// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	stdLog "log"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/ajg/form"
	"github.com/codegangsta/negroni"
	"github.com/nu7hatch/gouuid"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/cmd"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/log"
)

const (
	tsuruMin      = "1.0.1"
	craneMin      = "1.0.0"
	tsuruAdminMin = "1.0.0"
)

func validate(token string, r *http.Request) (auth.Token, error) {
	t, err := app.AuthScheme.Auth(token)
	if err != nil {
		t, err = auth.APIAuth(token)
		if err != nil {
			return nil, err
		}
	}
	if t.IsAppToken() {
		if q := r.URL.Query().Get(":app"); q != "" && t.GetAppName() != q {
			return nil, &tsuruErrors.HTTP{
				Code:    http.StatusForbidden,
				Message: fmt.Sprintf("app token mismatch, token for %q, request for %q", t.GetAppName(), q),
			}
		}
	} else {
		if q := r.URL.Query().Get(":app"); q != "" {
			_, err = getAppFromContext(q, r)
			if err != nil {
				return nil, err
			}
		}
	}
	return t, nil
}

func contextClearerMiddleware(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	defer context.Clear(r)
	next(w, r)
}

func flushingWriterMiddleware(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	defer func() {
		if r.Body != nil {
			r.Body.Close()
		}
	}()
	fw := io.FlushingWriter{ResponseWriter: w}
	next(&fw, r)
}

func setRequestIDHeaderMiddleware(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	requestIDHeader, _ := config.GetString("request-id-header")
	if requestIDHeader == "" {
		next(w, r)
		return
	}
	requestID := r.Header.Get(requestIDHeader)
	if requestID == "" {
		unparsedID, err := uuid.NewV4()
		if err != nil {
			log.Errorf("unable to generate request id: %s", err)
			next(w, r)
			return
		}
		requestID = unparsedID.String()
	}
	context.SetRequestID(r, requestIDHeader, requestID)
	next(w, r)
}

func setVersionHeadersMiddleware(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	w.Header().Set("Supported-Tsuru", tsuruMin)
	w.Header().Set("Supported-Crane", craneMin)
	w.Header().Set("Supported-Tsuru-Admin", tsuruAdminMin)
	next(w, r)
}

func errorHandlingMiddleware(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	next(w, r)
	err := context.GetRequestError(r)
	if err != nil {
		verbosity, _ := strconv.Atoi(r.Header.Get(cmd.VerbosityHeader))
		code := http.StatusInternalServerError
		switch t := errors.Cause(err).(type) {
		case *tsuruErrors.ValidationError:
			code = http.StatusBadRequest
		case *tsuruErrors.HTTP:
			code = t.Code
		}
		if verbosity == 0 {
			err = fmt.Errorf("%s", err)
		} else {
			err = fmt.Errorf("%+v", err)
		}
		flushing, ok := w.(*io.FlushingWriter)
		if ok && flushing.Wrote() {
			if w.Header().Get("Content-Type") == "application/x-json-stream" {
				data, marshalErr := json.Marshal(io.SimpleJsonMessage{Error: err.Error()})
				if marshalErr == nil {
					w.Write(append(data, "\n"...))
				}
			} else {
				fmt.Fprintln(w, err)
			}
		} else {
			http.Error(w, err.Error(), code)
		}
		log.Errorf("failure running HTTP request %s %s (%d): %s", r.Method, r.URL.Path, code, err)
	}
}

func authTokenMiddleware(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	token := r.Header.Get("Authorization")
	if token != "" {
		t, err := validate(token, r)
		if err != nil {
			if err != auth.ErrInvalidToken {
				context.AddRequestError(r, err)
				return
			}
			log.Debugf("Ignored invalid token for %s: %s", r.URL.Path, err.Error())
		} else {
			context.SetAuthToken(r, t)
		}
	}
	next(w, r)
}

type appLockMiddleware struct {
	excludedHandlers []http.Handler
}

var lockWaitDuration time.Duration = 10 * time.Second

func (m *appLockMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	if r.Method == "GET" {
		next(w, r)
		return
	}
	currentHandler := context.GetDelayedHandler(r)
	if currentHandler != nil {
		currentHandlerPtr := reflect.ValueOf(currentHandler).Pointer()
		for _, h := range m.excludedHandlers {
			if reflect.ValueOf(h).Pointer() == currentHandlerPtr {
				next(w, r)
				return
			}
		}
	}
	appName := r.URL.Query().Get(":app")
	if appName == "" {
		appName = r.URL.Query().Get(":appname")
	}
	if appName == "" {
		next(w, r)
		return
	}
	t := context.GetAuthToken(r)
	var owner string
	if t != nil {
		if t.IsAppToken() {
			owner = t.GetAppName()
		} else {
			owner = t.GetUserName()
		}
	}
	_, err := app.GetByName(appName)
	if err == app.ErrAppNotFound {
		context.AddRequestError(r, &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: err.Error()})
		return
	}
	ok, err := app.AcquireApplicationLockWait(appName, owner, fmt.Sprintf("%s %s", r.Method, r.URL.Path), lockWaitDuration)
	if err != nil {
		context.AddRequestError(r, errors.Wrap(err, "Error trying to acquire application lock"))
		return
	}
	if ok {
		defer func() {
			if !context.IsPreventUnlock(r) {
				app.ReleaseApplicationLock(appName)
			}
		}()
		next(w, r)
		return
	}
	a, err := app.GetByName(appName)
	if err != nil {
		if err == app.ErrAppNotFound {
			err = &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		} else {
			err = errors.Wrap(err, "Error to get application")
		}
	} else {
		httpErr := &tsuruErrors.HTTP{Code: http.StatusConflict}
		if a.Lock.Locked {
			httpErr.Message = a.Lock.String()
		} else {
			httpErr.Message = "Not locked anymore, please try again."
		}
		err = httpErr
	}
	context.AddRequestError(r, err)
}

func runDelayedHandler(w http.ResponseWriter, r *http.Request) {
	h := context.GetDelayedHandler(r)
	if h != nil {
		h.ServeHTTP(w, r)
	}
}

type loggerMiddleware struct {
	logger *stdLog.Logger
}

func newLoggerMiddleware() *loggerMiddleware {
	return &loggerMiddleware{
		logger: stdLog.New(os.Stdout, "", 0),
	}
}

func (l *loggerMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	start := time.Now()
	next(rw, r)
	duration := time.Since(start)
	statusCode := rw.(negroni.ResponseWriter).Status()
	if statusCode == 0 {
		statusCode = 200
	}
	nowFormatted := time.Now().Format(time.RFC3339Nano)
	requestIDHeader, _ := config.GetString("request-id-header")
	var requestID string
	if requestIDHeader != "" {
		requestID = context.GetRequestID(r, requestIDHeader)
		if requestID != "" {
			requestID = fmt.Sprintf(" [%s: %s]", requestIDHeader, requestID)
		}
	}
	l.logger.Printf("%s %s %s %d in %0.6fms%s", nowFormatted, r.Method, r.URL.Path, statusCode, float64(duration)/float64(time.Millisecond), requestID)
}

func contentHijacker(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	defer next(w, r)
	if r.Body == nil {
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		return
	}
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Errorf("[content hijacker] error reading body: %v", err)
		return
	}
	if len(data) == 0 {
		return
	}
	var bodyData map[string]interface{}
	err = json.Unmarshal(data, &bodyData)
	if err != nil {
		log.Errorf("[content hijacker] unable to parse as json: %q - %v", string(data), err)
		return
	}
	formData, err := form.EncodeToString(bodyData)
	if err != nil {
		log.Errorf("[content hijacker] unable to format as form: %v", err)
		return
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Body = ioutil.NopCloser(strings.NewReader(formData))
}
