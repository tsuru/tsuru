// AUTOMATICALLY GENERATED FILE - DO NOT EDIT!
// Please run 'go generate' to update this file.
//
// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/tsuru/tsuru/router"
)

func toSupportedInterface(base *apiRouter, supports map[capability]bool) router.Router {
	apiRouterWithTLSSupportInst := &apiRouterWithTLSSupport{base}

	if !supports["tls"] {
		return &struct {
			router.Router
		}{
			base,
		}
	}
	if supports["tls"] {
		return &struct {
			router.Router
			router.TLSRouter
		}{
			base,
			apiRouterWithTLSSupportInst,
		}
	}
	return nil
}
