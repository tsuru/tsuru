// AUTOMATICALLY GENERATED FILE - DO NOT EDIT!
// Please run 'go generate' to update this file.
//
// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/tsuru/tsuru/router"
)

func toSupportedInterface(base *apiRouter, supports map[capability]bool) router.Router {
	apiRouterWithCnameSupportInst := &apiRouterWithCnameSupport{base}
	apiRouterWithHealthcheckSupportInst := &apiRouterWithHealthcheckSupport{base}
	apiRouterWithTLSSupportInst := &apiRouterWithTLSSupport{base}

	if !supports["cname"] && !supports["healthcheck"] && !supports["tls"] {
		return &struct {
			router.Router
		}{
			base,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["tls"] {
		return &struct {
			router.Router
			router.CNameRouter
		}{
			base,
			apiRouterWithCnameSupportInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["tls"] {
		return &struct {
			router.Router
			router.CustomHealthcheckRouter
		}{
			base,
			apiRouterWithHealthcheckSupportInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["tls"] {
		return &struct {
			router.Router
			router.CNameRouter
			router.CustomHealthcheckRouter
		}{
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["tls"] {
		return &struct {
			router.Router
			router.TLSRouter
		}{
			base,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["tls"] {
		return &struct {
			router.Router
			router.CNameRouter
			router.TLSRouter
		}{
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["tls"] {
		return &struct {
			router.Router
			router.CustomHealthcheckRouter
			router.TLSRouter
		}{
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["tls"] {
		return &struct {
			router.Router
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.TLSRouter
		}{
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithTLSSupportInst,
		}
	}
	return nil
}
