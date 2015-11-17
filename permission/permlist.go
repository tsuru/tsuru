// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

//go:generate go run ./generator/main.go -o permitems.go

var PermissionRegistry = (&registry{}).addWithCtx(
	"app", []contextType{CtxApp, CtxTeam, CtxPool},
).addWithCtx(
	"app.create", []contextType{CtxTeam, CtxPool},
).add(
	"app.update.env.set",
	"app.update.env.unset",
	"app.update.restart",
	"app.update.teamowner",
	"app.deploy",
	"app.read",
	"app.delete",
).addWithCtx(
	"node", []contextType{CtxPool},
).add(
	"node.create",
	"node.read",
	"node.update",
	"node.delete",
).addWithCtx(
	"iaas.read", []contextType{CtxIaaS},
).addWithCtx(
	"team", []contextType{CtxTeam},
).addWithCtx(
	"team.create", []contextType{},
).add(
	"team.delete",
	"team.update.add-member",
	"team.update.remove-member",
).add(
	"user.create",
	"user.delete",
	"user.list",
	"user.update",
).addWithCtx(
	"service-instance", []contextType{CtxServiceInstance, CtxTeam},
).addWithCtx(
	"service-instance.create", []contextType{},
).add(
	"service-instance.read",
	"service-instance.delete",
	"service-instance.update.bind",
	"service-instance.update.unbind",
	"service-instance.update.grant",
	"service-instance.update.revoke",
).add(
	"role.create",
	"role.delete",
	"role.update",
	"role.assign",
)
