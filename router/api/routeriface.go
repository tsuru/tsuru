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
	apiRouterWithInfoInst := &apiRouterWithInfo{base}
	apiRouterWithStatusInst := &apiRouterWithStatus{base}
	apiRouterWithTLSSupportInst := &apiRouterWithTLSSupport{base}

	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
		}{
			base,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.CNameRouter
		}{
			base,
			apiRouterWithCnameSupportInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.CustomHealthcheckRouter
		}{
			base,
			apiRouterWithHealthcheckSupportInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["status"] && !supports["tls"] {
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
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.InfoRouter
		}{
			base,
			apiRouterWithInfoInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.CNameRouter
			router.InfoRouter
		}{
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.CustomHealthcheckRouter
			router.InfoRouter
		}{
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
		}{
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.StatusRouter
		}{
			base,
			apiRouterWithStatusInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.CNameRouter
			router.StatusRouter
		}{
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithStatusInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.CustomHealthcheckRouter
			router.StatusRouter
		}{
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithStatusInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.StatusRouter
		}{
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithStatusInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.InfoRouter
			router.StatusRouter
		}{
			base,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.CNameRouter
			router.InfoRouter
			router.StatusRouter
		}{
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.StatusRouter
		}{
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && supports["status"] && !supports["tls"] {
		return &struct {
			router.Router
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.StatusRouter
		}{
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
			router.TLSRouter
		}{
			base,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["status"] && supports["tls"] {
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
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["status"] && supports["tls"] {
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
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["status"] && supports["tls"] {
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
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.InfoRouter
			router.TLSRouter
		}{
			base,
			apiRouterWithInfoInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.CNameRouter
			router.InfoRouter
			router.TLSRouter
		}{
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.TLSRouter
		}{
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.TLSRouter
		}{
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
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.CNameRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.CustomHealthcheckRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.StatusRouter
			router.TLSRouter
		}{
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
			router.InfoRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["status"] && supports["tls"] {
		return &struct {
			router.Router
			router.CNameRouter
			router.InfoRouter
			router.StatusRouter
			router.TLSRouter
		}{
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
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.StatusRouter
			router.TLSRouter
		}{
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
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.StatusRouter
			router.TLSRouter
		}{
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
