// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	stdContext "context"
	"encoding/json"
	"fmt"
	stdIO "io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cezarsa/form"
	"github.com/codegangsta/negroni"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/api/context"
	"github.com/tsuru/tsuru/api/observability"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/peer"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/log"
	tsuruNet "github.com/tsuru/tsuru/net"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/set"
	appTypes "github.com/tsuru/tsuru/types/app"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	tsuruMin = "1.0.1"

	defaultMaxMemory = 32 << 20 // 32 MB

	promNamespace = "tsuru"
	promSubsystem = "api"

	verbosityHeader = "X-Tsuru-Verbosity"
)

var (
	tokenValidateTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "token_valid_total",
		Help:      "The number of successful validation of tokens",
	}, []string{"engine"})

	tokenInvalidTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: promSubsystem,
		Name:      "token_invalid_total",
		Help:      "The number of unsuccessful validation of tokens",
	})
)

func validate(token string, r *http.Request) (auth.Token, error) {
	var t auth.Token
	t, err := tokenByAllAuthEngines(r.Context(), token)
	if err != nil {
		return nil, err
	}

	tokenValidateTotal.WithLabelValues(t.Engine()).Inc()

	span := trace.SpanFromContext(r.Context())

	if span != nil && span.IsRecording() {
		span.SetAttributes(attribute.String("user.name", t.GetUserName()))
	}
	if q := r.URL.Query().Get(":app"); q != "" {
		_, err = getAppFromContext(q, r)
		if err != nil {
			return nil, err
		}
	}

	return t, nil
}

func tokenByAllAuthEngines(ctx stdContext.Context, token string) (auth.Token, error) {
	t, err := app.AuthScheme.Auth(ctx, token)
	if err == nil {
		return t, nil
	}

	t, err = auth.APIAuth(ctx, token)
	if err == nil {
		return t, nil
	}

	t, err = servicemanager.TeamToken.Authenticate(ctx, token)
	if err == nil {
		return t, nil
	}

	t, err = peer.Auth(ctx, token)
	if err == nil {
		return t, nil
	}

	tokenInvalidTotal.Inc()

	return nil, err
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
		unparsedID, err := uuid.NewRandom()
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
	next(w, r)
}

func errorHandlingMiddleware(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	next(w, r)
	err := context.GetRequestError(r)
	if err != nil {
		verbosity, _ := strconv.Atoi(r.Header.Get(verbosityHeader))
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
		defer func() {
			if rw, ok := w.(negroni.ResponseWriter); ok {
				observability.FinishSpan(r, rw.Status())
			} else {
				observability.FinishSpan(r, http.StatusOK)
			}
		}()
		h.ServeHTTP(w, r)
	}
}

func InputValues(r *http.Request, field string) ([]string, bool) {
	parseForm(r)
	switch getContentType(r) {
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
	switch getContentType(r) {
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

func ParseJSON(r *http.Request, dst interface{}) error {
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

	return nil
}

func ParseInput(r *http.Request, dst interface{}) error {
	contentType := getContentType(r)
	switch contentType {
	case "application/json":
		return ParseJSON(r, dst)
	default:
		dec := form.NewDecoder(nil)
		dec.IgnoreCase(true)
		dec.IgnoreUnknownKeys(true)
		dec.UseJSONTags(false)
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
	contentType := getContentType(r)
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

func getContentType(r *http.Request) string {
	parts := strings.Split(r.Header.Get("Content-Type"), ";")
	return parts[0]
}
