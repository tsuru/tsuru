// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

//go:generate bash -c "rm -f permitems.go && go run ./generator/main.go -o permitems.go"

var PermissionRegistry = (&registry{}).addWithCtx(
	"app", []contextType{CtxApp, CtxTeam, CtxPool},
).addWithCtx(
	"app.create", []contextType{CtxTeam},
).add(
	"app.update.log",
	"app.update.pool",
	"app.update.unit.add",
	"app.update.unit.remove",
	"app.update.unit.register",
	"app.update.unit.status",
	"app.update.env.set",
	"app.update.env.unset",
	"app.update.restart",
	"app.update.start",
	"app.update.stop",
	"app.update.swap",
	"app.update.grant",
	"app.update.revoke",
	"app.update.teamowner",
	"app.update.cname.add",
	"app.update.cname.remove",
	"app.update.plan",
	"app.update.bind",
	"app.update.unbind",
	"app.deploy",
	"app.deploy.rollback",
	"app.read",
	"app.read.deploy",
	"app.read.env",
	"app.read.metric",
	"app.read.log",
	"app.delete",
	"app.run",
	"app.admin.unlock",
	"app.admin.routes",
	"app.admin.quota",
).addWithCtx(
	"node", []contextType{CtxPool},
).add(
	"node.create",
	"node.read",
	"node.update",
	"node.delete",
	"node.bs",
	"node.autoscale",
).addWithCtx(
	"machine", []contextType{CtxIaaS},
).add(
	"machine.create",
	"machine.delete",
	"machine.read",
	"machine.template.create",
	"machine.template.delete",
	"machine.template.update",
	"machine.template.read",
).addWithCtx(
	"team", []contextType{CtxTeam},
).addWithCtx(
	"team.create", []contextType{},
).add(
	"team.delete",
).add(
	"user.create",
	"user.delete",
	"user.update.token",
	"user.update.quota",
).addWithCtx(
	"service", []contextType{CtxService, CtxTeam},
).addWithCtx(
	"service.create", []contextType{CtxTeam},
).add(
	"service.read.doc",
	"service.read.plans",
	"service.update.proxy",
	"service.update.revoke-access",
	"service.update.grant-access",
	"service.update.doc",
	"service.delete",
).addWithCtx(
	"service-instance", []contextType{CtxServiceInstance, CtxTeam},
).addWithCtx(
	"service-instance.create", []contextType{CtxTeam},
).add(
	"service-instance.read.status",
	"service-instance.delete",
	"service-instance.update.proxy",
	"service-instance.update.bind",
	"service-instance.update.unbind",
	"service-instance.update.grant",
	"service-instance.update.revoke",
).add(
	"role.create",
	"role.delete",
	"role.update.assign",
	"role.update.dissociate",
	"role.default.create",
	"role.default.delete",
).add(
	"platform.create",
	"platform.delete",
	"platform.update",
).add(
	"plan.create",
	"plan.delete",
).add(
	"pool.create",
	"pool.update",
	"pool.delete",
).add(
	"debug",
).add(
	"healing",
)
