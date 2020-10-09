// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	stdIO "io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ajg/form"
	uuid "github.com/nu7hatch/gouuid"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/cmd"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/set"
	appTypes "github.com/tsuru/tsuru/types/app"
)

const (
	tsuruMin      = "1.0.1"
	craneMin      = "1.0.0"
	tsuruAdminMin = "1.0.0"

	defaultMaxMemory = 32 << 20 // 32 MB
)

func validate(token string, r *http.Request) (auth.Token, error) {
	var t auth.Token
	t, err := app.AuthScheme.Auth(r.Context(), token)
	if err != nil {
		t, err = auth.APIAuth(token)
		if err != nil {
			t, err = servicemanager.TeamToken.Authenticate(r.Context(), token)
			if err != nil {
				return nil, err
			}
		}
	}
	span := opentracing.SpanFromContext(r.Context())

	if t.IsAppToken() {
		tokenAppName := t.GetAppName()
		if span != nil {
			span.SetTag("app.name", tokenAppName)
		}
		if q := r.URL.Query().Get(":app"); q != "" && tokenAppName != q {
			return nil, &tsuruErrors.HTTP{
				Code:    http.StatusForbidden,
				Message: fmt.Sprintf("app token mismatch, token for %q, request for %q", t.GetAppName(), q),
			}
		}
	} else {
		if span != nil {
			span.SetTag("user.name", t.GetUserName())
		}
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

// contextNoCancelMiddleware replaces the original request context with a
// non-cancelable context. This allows tsuru to retain its legacy behavior of
// not canceling an operation on connection failures.
func contextNoCancelMiddleware(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	r = r.WithContext(tsuruNet.WithoutCancel(r.Context()))
	next(w, r)
}

type flushingWriterMiddleware struct {
	latencyConfig map[string]time.Duration
}

func (m *flushingWriterMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	defer func() {
		if r.Body != nil {
			r.Body.Close()
		}
	}()
	if flusher, ok := w.(io.WriterFlusher); ok {
		flushingWriter := &io.FlushingWriter{WriterFlusher: flusher}
		defer flushingWriter.Close()
		if m.latencyConfig != nil {
			flushingWriter.MaxLatency = m.latencyConfig[r.URL.Query().Get(":mux-route-name")]
		}
		w = flushingWriter
	}
	next(w, r)
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
		if errors.Cause(err) == appTypes.ErrAppNotFound {
			code = http.StatusNotFound
		}
		if verbosity == 0 {
			err = fmt.Errorf("%s", err)
		} else {
			err = fmt.Errorf("%+v", err)
		}
		flushing, ok := w.(*io.FlushingWriter)
		if ok && flushing.Wrote() {
			if w.Header().Get("Content-Type") == "application/x-json-stream" {
				data, marshalErr := json.Marshal(io.SimpleJsonMessage{Error: err.Error(), Timestamp: time.Now().UTC()})
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

func runDelayedHandler(w http.ResponseWriter, r *http.Request) {
	h := context.GetDelayedHandler(r)
	if h != nil {
		h.ServeHTTP(w, r)
	}
}

func InputValues(r *http.Request, field string) ([]string, bool) {
	parseForm(r)
	switch r.Header.Get("Content-Type") {
	case "application/json":
		data, err := context.GetBody(r)
		if err != nil {
			break
		}
		if len(data) == 0 {
			break
		}
		var dst map[string]interface{}
		err = json.Unmarshal(data, &dst)
		if err != nil {
			break
		}
		val, isSet := dst[field]
		if !isSet {
			break
		}
		if _, isSet := r.Form[field]; !isSet {
			r.Form[field] = nil
		}
		if asSlice, ok := val.([]interface{}); ok {
			for _, v := range asSlice {
				r.Form[field] = append(r.Form[field], fmt.Sprint(v))
			}
		} else {
			r.Form[field] = append(r.Form[field], fmt.Sprint(val))
		}
	}
	val, isSet := r.Form[field]
	return val, isSet
}

func InputValue(r *http.Request, field string) string {
	if values, _ := InputValues(r, field); len(values) > 0 {
		return values[0]
	}
	return ""
}

func InputFields(r *http.Request, exclude ...string) url.Values {
	parseForm(r)
	baseValues := r.Form
	excludeSet := set.FromSlice(exclude)
	switch r.Header.Get("Content-Type") {
	case "application/json":
		data, err := context.GetBody(r)
		if err != nil {
			return nil
		}
		if len(data) == 0 {
			return nil
		}
		var dst map[string]interface{}
		err = json.Unmarshal(data, &dst)
		if err != nil {
			return nil
		}
		bodyValues, err := form.EncodeToValues(dst)
		if err != nil {
			return nil
		}
		for key, values := range bodyValues {
			for _, v := range values {
				baseValues.Add(key, v)
			}
		}
	}
	ret := url.Values{}
	for key, value := range baseValues {
		if excludeSet.Includes(key) {
			ret[key] = []string{"*****"}
			continue
		}
		ret[key] = value
	}
	return ret
}

func ParseInput(r *http.Request, dst interface{}) error {
	contentType := r.Header.Get("Content-Type")
	switch contentType {
	case "application/json":
		data, err := context.GetBody(r)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return nil
		}
		err = json.Unmarshal(data, dst)
		if err != nil {
			return &tsuruErrors.HTTP{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("unable to parse as json: %q - %v", string(data), err),
			}
		}
	default:
		dec := form.NewDecoder(nil)
		dec.IgnoreCase(true)
		dec.IgnoreUnknownKeys(true)
		err := parseForm(r)
		if err != nil && contentType == "application/x-www-form-urlencoded" {
			return &tsuruErrors.HTTP{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("unable to parse form: %v", err),
			}
		}
		err = dec.DecodeValues(dst, r.Form)
		if err != nil {
			return &tsuruErrors.HTTP{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("unable to decode form: %#v - %v", r.Form, err),
			}
		}
	}
	return nil
}

func parseForm(r *http.Request) error {
	if r.Form != nil {
		return nil
	}
	var buf bytes.Buffer
	var readCloser struct {
		stdIO.Reader
		stdIO.Closer
	}
	if r.Body != nil {
		readCloser.Reader = stdIO.TeeReader(r.Body, &buf)
		readCloser.Closer = r.Body
		r.Body = &readCloser
	}
	contentType := r.Header.Get("Content-Type")
	var err error
	if strings.HasPrefix(contentType, "multipart") {
		err = r.ParseMultipartForm(defaultMaxMemory)
	} else {
		err = r.ParseForm()
	}
	if buf.Len() > 0 {
		readCloser.Reader = &buf
	}
	return err
}
