// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package context

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
)

type ctxKey int

type reqIDHeaderCtxKey string

const (
	tokenContextKey ctxKey = iota
	errorContextKey
	delayedHandlerKey
	preventUnlockKey
	appContextKey
	reqBodyKey
)

func Clear(r *http.Request) {
	if r == nil {
		return
	}
	newReq := r.WithContext(context.Background())
	*r = *newReq
}

func GetBody(r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, nil
	}
	if v, ok := r.Context().Value(reqBodyKey).([]byte); ok {
		return v, nil
	}
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	newReq := r.WithContext(context.WithValue(r.Context(), reqBodyKey, data))
	*r = *newReq
	r.Body = ioutil.NopCloser(bytes.NewReader(data))
	return data, nil
}

func GetApp(r *http.Request) *app.App {
	if r == nil {
		return nil
	}
	if v, ok := r.Context().Value(appContextKey).(*app.App); ok {
		return v
	}
	return nil
}

func SetApp(r *http.Request, a *app.App) {
	newReq := r.WithContext(context.WithValue(r.Context(), appContextKey, a))
	*r = *newReq
}

func GetAuthToken(r *http.Request) auth.Token {
	if r == nil {
		return nil
	}
	if v, ok := r.Context().Value(tokenContextKey).(auth.Token); ok {
		return v
	}
	return nil
}

func SetAuthToken(r *http.Request, t auth.Token) {
	newReq := r.WithContext(context.WithValue(r.Context(), tokenContextKey, t))
	*r = *newReq
}

func AddRequestError(r *http.Request, err error) {
	if err == nil {
		return
	}
	existingErr := r.Context().Value(errorContextKey)
	if existingErr != nil {
		err = &errors.CompositeError{Base: existingErr.(error), Message: err.Error()}
	}
	newReq := r.WithContext(context.WithValue(r.Context(), errorContextKey, err))
	*r = *newReq
}

func GetRequestError(r *http.Request) error {
	if r == nil {
		return nil
	}
	if v, ok := r.Context().Value(errorContextKey).(error); ok {
		return v
	}
	return nil
}

func SetDelayedHandler(r *http.Request, h http.Handler) {
	newReq := r.WithContext(context.WithValue(r.Context(), delayedHandlerKey, h))
	*r = *newReq
}

func GetDelayedHandler(r *http.Request) http.Handler {
	if r == nil {
		return nil
	}
	if v, ok := r.Context().Value(delayedHandlerKey).(http.Handler); ok {
		return v
	}
	return nil
}

func SetPreventUnlock(r *http.Request) {
	newReq := r.WithContext(context.WithValue(r.Context(), preventUnlockKey, true))
	*r = *newReq
}

func IsPreventUnlock(r *http.Request) bool {
	if r == nil {
		return false
	}
	if v, ok := r.Context().Value(preventUnlockKey).(bool); ok {
		return v
	}
	return false
}

func SetRequestID(r *http.Request, requestIDHeader, requestID string) {
	newReq := r.WithContext(context.WithValue(r.Context(), reqIDHeaderCtxKey(requestIDHeader), requestID))
	*r = *newReq
}

func GetRequestID(r *http.Request, requestIDHeader string) string {
	if r == nil {
		return ""
	}
	requestID := r.Context().Value(reqIDHeaderCtxKey(requestIDHeader))
	if requestID == nil {
		return ""
	}
	return requestID.(string)
}
