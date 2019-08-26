// AUTOMATICALLY GENERATED FILE - DO NOT EDIT!
// Please run 'go generate' to update this file.
//
// Copyright 2019 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/tsuru/tsuru/router"
)

func toSupportedInterface(base *apiRouter, supports map[capability]bool) router.Router {
	apiRouterWithCnameSupportInst := &apiRouterWithCnameSupport{base}
	apiRouterWithHealthcheckSupportInst := &apiRouterWithHealthcheckSupport{base}
	apiRouterWithInfoInst := &apiRouterWithInfo{base}
	apiRouterWithStatusInst := &apiRouterWithStatus{base}
	apiRouterWithTLSSupportInst := &apiRouterWithTLSSupport{base}

	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
		}{
			base,
			base,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.InfoRouter
		}{
			base,
			base,
			apiRouterWithInfoInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.InfoRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.StatusRouter
		}{
			base,
			base,
			apiRouterWithStatusInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.StatusRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithStatusInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.StatusRouter
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithStatusInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.StatusRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithStatusInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.InfoRouter
			router.StatusRouter
		}{
			base,
			base,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.InfoRouter
			router.StatusRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.StatusRouter
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.StatusRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.InfoRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithInfoInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.InfoRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.InfoRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.InfoRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	return nil
}
