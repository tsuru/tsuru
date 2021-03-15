// AUTOMATICALLY GENERATED FILE - DO NOT EDIT!
// Please run 'go generate' to update this file.
//
// Copyright 2021 tsuru authors. All rights reserved.
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
	apiRouterWithPrefixInst := &apiRouterWithPrefix{base}
	apiRouterWithStatusInst := &apiRouterWithStatus{base}
	apiRouterWithTLSSupportInst := &apiRouterWithTLSSupport{base}
	apiRouterV2Inst := &apiRouterV2{base}

	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["prefix"] && !supports["status"] && !supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
		}{
			base,
			base,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["prefix"] && !supports["status"] && !supports["tls"] && !supports["v2"] {
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
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["prefix"] && !supports["status"] && !supports["tls"] && !supports["v2"] {
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
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["prefix"] && !supports["status"] && !supports["tls"] && !supports["v2"] {
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
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["prefix"] && !supports["status"] && !supports["tls"] && !supports["v2"] {
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
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["prefix"] && !supports["status"] && !supports["tls"] && !supports["v2"] {
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
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["prefix"] && !supports["status"] && !supports["tls"] && !supports["v2"] {
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
	if supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["prefix"] && !supports["status"] && !supports["tls"] && !supports["v2"] {
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
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["prefix"] && !supports["status"] && !supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.PrefixRouter
		}{
			base,
			base,
			apiRouterWithPrefixInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["prefix"] && !supports["status"] && !supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.PrefixRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithPrefixInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["prefix"] && !supports["status"] && !supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.PrefixRouter
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithPrefixInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["prefix"] && !supports["status"] && !supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.PrefixRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithPrefixInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["prefix"] && !supports["status"] && !supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.InfoRouter
			router.PrefixRouter
		}{
			base,
			base,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["prefix"] && !supports["status"] && !supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.InfoRouter
			router.PrefixRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && supports["prefix"] && !supports["status"] && !supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.PrefixRouter
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && supports["prefix"] && !supports["status"] && !supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.PrefixRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["prefix"] && supports["status"] && !supports["tls"] && !supports["v2"] {
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
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["prefix"] && supports["status"] && !supports["tls"] && !supports["v2"] {
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
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["prefix"] && supports["status"] && !supports["tls"] && !supports["v2"] {
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
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["prefix"] && supports["status"] && !supports["tls"] && !supports["v2"] {
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
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["prefix"] && supports["status"] && !supports["tls"] && !supports["v2"] {
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
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["prefix"] && supports["status"] && !supports["tls"] && !supports["v2"] {
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
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["prefix"] && supports["status"] && !supports["tls"] && !supports["v2"] {
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
	if supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["prefix"] && supports["status"] && !supports["tls"] && !supports["v2"] {
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
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["prefix"] && supports["status"] && !supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.PrefixRouter
			router.StatusRouter
		}{
			base,
			base,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["prefix"] && supports["status"] && !supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.PrefixRouter
			router.StatusRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["prefix"] && supports["status"] && !supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.PrefixRouter
			router.StatusRouter
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["prefix"] && supports["status"] && !supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.PrefixRouter
			router.StatusRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["prefix"] && supports["status"] && !supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.InfoRouter
			router.PrefixRouter
			router.StatusRouter
		}{
			base,
			base,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["prefix"] && supports["status"] && !supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.InfoRouter
			router.PrefixRouter
			router.StatusRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && supports["prefix"] && supports["status"] && !supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.PrefixRouter
			router.StatusRouter
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && supports["prefix"] && supports["status"] && !supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.PrefixRouter
			router.StatusRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["prefix"] && !supports["status"] && supports["tls"] && !supports["v2"] {
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
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["prefix"] && !supports["status"] && supports["tls"] && !supports["v2"] {
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
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["prefix"] && !supports["status"] && supports["tls"] && !supports["v2"] {
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
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["prefix"] && !supports["status"] && supports["tls"] && !supports["v2"] {
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
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["prefix"] && !supports["status"] && supports["tls"] && !supports["v2"] {
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
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["prefix"] && !supports["status"] && supports["tls"] && !supports["v2"] {
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
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["prefix"] && !supports["status"] && supports["tls"] && !supports["v2"] {
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
	if supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["prefix"] && !supports["status"] && supports["tls"] && !supports["v2"] {
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
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["prefix"] && !supports["status"] && supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.PrefixRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithPrefixInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["prefix"] && !supports["status"] && supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.PrefixRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["prefix"] && !supports["status"] && supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.PrefixRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["prefix"] && !supports["status"] && supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.PrefixRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["prefix"] && !supports["status"] && supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.InfoRouter
			router.PrefixRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["prefix"] && !supports["status"] && supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.InfoRouter
			router.PrefixRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && supports["prefix"] && !supports["status"] && supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.PrefixRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && supports["prefix"] && !supports["status"] && supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.PrefixRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["prefix"] && supports["status"] && supports["tls"] && !supports["v2"] {
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
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["prefix"] && supports["status"] && supports["tls"] && !supports["v2"] {
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
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["prefix"] && supports["status"] && supports["tls"] && !supports["v2"] {
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
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["prefix"] && supports["status"] && supports["tls"] && !supports["v2"] {
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
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["prefix"] && supports["status"] && supports["tls"] && !supports["v2"] {
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
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["prefix"] && supports["status"] && supports["tls"] && !supports["v2"] {
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
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["prefix"] && supports["status"] && supports["tls"] && !supports["v2"] {
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
	if supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["prefix"] && supports["status"] && supports["tls"] && !supports["v2"] {
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
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["prefix"] && supports["status"] && supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.PrefixRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["prefix"] && supports["status"] && supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.PrefixRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["prefix"] && supports["status"] && supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.PrefixRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["prefix"] && supports["status"] && supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.PrefixRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["prefix"] && supports["status"] && supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.InfoRouter
			router.PrefixRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["prefix"] && supports["status"] && supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.InfoRouter
			router.PrefixRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && supports["prefix"] && supports["status"] && supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.PrefixRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && supports["prefix"] && supports["status"] && supports["tls"] && !supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.PrefixRouter
			router.StatusRouter
			router.TLSRouter
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["prefix"] && !supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["prefix"] && !supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["prefix"] && !supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["prefix"] && !supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["prefix"] && !supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.InfoRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithInfoInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["prefix"] && !supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.InfoRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["prefix"] && !supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["prefix"] && !supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["prefix"] && !supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.PrefixRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithPrefixInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["prefix"] && !supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.PrefixRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithPrefixInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["prefix"] && !supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.PrefixRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithPrefixInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["prefix"] && !supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.PrefixRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithPrefixInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["prefix"] && !supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.InfoRouter
			router.PrefixRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["prefix"] && !supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.InfoRouter
			router.PrefixRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && supports["prefix"] && !supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.PrefixRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && supports["prefix"] && !supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.PrefixRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["prefix"] && supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.StatusRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithStatusInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["prefix"] && supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.StatusRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithStatusInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["prefix"] && supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.StatusRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithStatusInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["prefix"] && supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.StatusRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithStatusInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["prefix"] && supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.InfoRouter
			router.StatusRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["prefix"] && supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.InfoRouter
			router.StatusRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["prefix"] && supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.StatusRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["prefix"] && supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.StatusRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["prefix"] && supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.PrefixRouter
			router.StatusRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["prefix"] && supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.PrefixRouter
			router.StatusRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["prefix"] && supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.PrefixRouter
			router.StatusRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["prefix"] && supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.PrefixRouter
			router.StatusRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["prefix"] && supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.InfoRouter
			router.PrefixRouter
			router.StatusRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["prefix"] && supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.InfoRouter
			router.PrefixRouter
			router.StatusRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && supports["prefix"] && supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.PrefixRouter
			router.StatusRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && supports["prefix"] && supports["status"] && !supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.PrefixRouter
			router.StatusRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["prefix"] && !supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["prefix"] && !supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["prefix"] && !supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["prefix"] && !supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["prefix"] && !supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.InfoRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithInfoInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["prefix"] && !supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.InfoRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["prefix"] && !supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["prefix"] && !supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["prefix"] && !supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.PrefixRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithPrefixInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["prefix"] && !supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.PrefixRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["prefix"] && !supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.PrefixRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["prefix"] && !supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.PrefixRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["prefix"] && !supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.InfoRouter
			router.PrefixRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["prefix"] && !supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.InfoRouter
			router.PrefixRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && supports["prefix"] && !supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.PrefixRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && supports["prefix"] && !supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.PrefixRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["prefix"] && supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.StatusRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && !supports["prefix"] && supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.StatusRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["prefix"] && supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.StatusRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && !supports["prefix"] && supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.StatusRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["prefix"] && supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.InfoRouter
			router.StatusRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && !supports["prefix"] && supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.InfoRouter
			router.StatusRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["prefix"] && supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.StatusRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && !supports["prefix"] && supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.StatusRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["prefix"] && supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.PrefixRouter
			router.StatusRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && !supports["info"] && supports["prefix"] && supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.PrefixRouter
			router.StatusRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["prefix"] && supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.PrefixRouter
			router.StatusRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && !supports["info"] && supports["prefix"] && supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.PrefixRouter
			router.StatusRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["prefix"] && supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.InfoRouter
			router.PrefixRouter
			router.StatusRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && !supports["healthcheck"] && supports["info"] && supports["prefix"] && supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.InfoRouter
			router.PrefixRouter
			router.StatusRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if !supports["cname"] && supports["healthcheck"] && supports["info"] && supports["prefix"] && supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.PrefixRouter
			router.StatusRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	if supports["cname"] && supports["healthcheck"] && supports["info"] && supports["prefix"] && supports["status"] && supports["tls"] && supports["v2"] {
		return &struct {
			router.Router
			router.OptsRouter
			router.CNameRouter
			router.CustomHealthcheckRouter
			router.InfoRouter
			router.PrefixRouter
			router.StatusRouter
			router.TLSRouter
			router.RouterV2
		}{
			base,
			base,
			apiRouterWithCnameSupportInst,
			apiRouterWithHealthcheckSupportInst,
			apiRouterWithInfoInst,
			apiRouterWithPrefixInst,
			apiRouterWithStatusInst,
			apiRouterWithTLSSupportInst,
			apiRouterV2Inst,
		}
	}
	return nil
}
