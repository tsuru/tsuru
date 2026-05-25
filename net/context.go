// Copyright 2020 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package net

import "context"

type baseContextKey struct{}

var baseContextValue baseContextKey

func CancelableParentContext(ctx context.Context) context.Context {
	if ctx == nil {
		return ctx
	}
	if baseCtx, ok := ctx.Value(baseContextValue).(context.Context); ok {
		return baseCtx
	}
	return ctx
}

func WithoutCancel(ctx context.Context) context.Context {
	return context.WithValue(context.WithoutCancel(ctx), baseContextValue, ctx)
}
