// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

var (
	CtxGlobal          = ContextType("global")
	CtxApp             = ContextType("app")
	CtxTeam            = ContextType("team")
	CtxUser            = ContextType("user")
	CtxPool            = ContextType("pool")
	CtxIaaS            = ContextType("iaas")
	CtxService         = ContextType("service")
	CtxServiceInstance = ContextType("service-instance")
	CtxVolume          = ContextType("volume")

	ContextTypes = []ContextType{
		CtxGlobal, CtxApp, CtxTeam, CtxPool, CtxIaaS, CtxService, CtxServiceInstance,
	}
)

type ContextType string

type PermissionContext struct {
	CtxType ContextType
	Value   string
}
