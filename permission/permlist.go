// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	permTypes "github.com/tsuru/tsuru/types/permission"
)

//go:generate bash -c "rm -f permitems.go && go run ./generator/main.go -o permitems.go"

var PermissionRegistry = (&registry{}).addWithCtx(
	"app", []permTypes.ContextType{permTypes.CtxApp, permTypes.CtxTeam, permTypes.CtxPool},
).addWithCtx(
	"app.create", []permTypes.ContextType{permTypes.CtxTeam},
).add(
	"app.update.description",
	"app.update.tags",
	"app.update.log",
	"app.update.pool",
	"app.update.unit.add",
	"app.update.unit.remove",
	"app.update.unit.register",
	"app.update.unit.status",
	"app.update.unit.autoscale.add",
	"app.update.unit.autoscale.remove",
	"app.update.env.set",
	"app.update.env.unset",
	"app.update.restart",
	"app.update.sleep",
	"app.update.start",
	"app.update.stop",
	"app.update.swap",
	"app.update.grant",
	"app.update.revoke",
	"app.update.teamowner",
	"app.update.cname.add",
	"app.update.cname.remove",
	"app.update.plan",
	"app.update.planoverride",
	"app.update.platform",
	"app.update.bind",
	"app.update.bind-volume",
	"app.update.image-reset",
	"app.update.events",
	"app.update.unbind",
	"app.update.unbind-volume",
	"app.update.certificate.set",
	"app.update.certificate.unset",
	"app.update.deploy.rollback",
	"app.update.router.add",
	"app.update.router.update",
	"app.update.router.remove",
	"app.update.routable",
	"app.deploy",
	"app.deploy.archive-url",
	"app.deploy.build",
	"app.deploy.git",
	"app.deploy.image",
	"app.deploy.rollback",
	"app.deploy.upload",
	"app.read",
	"app.read.deploy",
	"app.read.router",
	"app.read.env",
	"app.read.events",
	"app.read.metric",
	"app.read.log",
	"app.read.certificate",
	"app.delete",
	"app.run",
	"app.run.shell",
	"app.admin.routes",
	"app.admin.quota",
	"app.build",
).addWithCtx(
	"node", []permTypes.ContextType{permTypes.CtxPool},
).add(
	"node.create",
	"node.read",
	"node.update.move.container",
	"node.update.move.containers",
	"node.update.rebalance",
	"node.delete",
).addWithCtx(
	"node.autoscale", []permTypes.ContextType{},
).add(
	"node.autoscale.update",
	"node.autoscale.update.run",
	"node.autoscale.read",
	"node.autoscale.delete",
).addWithCtx(
	"machine", []permTypes.ContextType{permTypes.CtxIaaS},
).add(
	"machine.delete",
	"machine.read",
	"machine.read.events",
	"machine.template.create",
	"machine.template.delete",
	"machine.template.update",
	"machine.template.read",
).addWithCtx(
	"team", []permTypes.ContextType{permTypes.CtxTeam},
).addWithCtx(
	"team.create", []permTypes.ContextType{},
).add(
	"team.read.events",
	"team.delete",
	"team.update",
	"team.token.read",
	"team.token.create",
	"team.token.delete",
	"team.token.update",
).addWithCtx(
	"user", []permTypes.ContextType{permTypes.CtxUser},
).addWithCtx(
	"user.create", []permTypes.ContextType{},
).add(
	"user.delete",
	"user.read.events",
	"user.read.quota",
	"user.update.token",
	"user.update.quota",
	"user.update.password",
	"user.update.reset",
	"user.update.key.add",
	"user.update.key.remove",
).addWithCtx(
	"service", []permTypes.ContextType{permTypes.CtxService, permTypes.CtxTeam},
).addWithCtx(
	"service.create", []permTypes.ContextType{permTypes.CtxTeam},
).add(
	"service.read.doc",
	"service.read.plans",
	"service.read.events",
	"service.update.proxy",
	"service.update.revoke-access",
	"service.update.grant-access",
	"service.update.doc",
	"service.delete",
	"service-broker.read",
	"service-broker.read.events",
	"service-broker.create",
	"service-broker.delete",
	"service-broker.update",
).addWithCtx(
	"service-instance", []permTypes.ContextType{permTypes.CtxServiceInstance, permTypes.CtxTeam},
).addWithCtx(
	"service-instance.create", []permTypes.ContextType{permTypes.CtxTeam},
).add(
	"service-instance.read.events",
	"service-instance.read.status",
	"service-instance.delete",
	"service-instance.update.proxy",
	"service-instance.update.bind",
	"service-instance.update.unbind",
	"service-instance.update.grant",
	"service-instance.update.revoke",
	"service-instance.update.description",
	"service-instance.update.tags",
	"service-instance.update.teamowner",
	"service-instance.update.plan",
	"service-instance.update.parameters",
).add(
	"role.create",
	"role.delete",
	"role.read.events",
	"role.update.name",
	"role.update.assign",
	"role.update.dissociate",
	"role.update.description",
	"role.update.context.type",
	"role.update.permission.add",
	"role.update.permission.remove",
	"role.default.create",
	"role.default.delete",
).add(
	"platform.create",
	"platform.delete",
	"platform.update",
	"platform.read.events",
).add(
	"plan.create",
	"plan.delete",
	"plan.read.events",
).addWithCtx(
	"pool", []permTypes.ContextType{permTypes.CtxPool},
).addWithCtx(
	"pool.create", []permTypes.ContextType{},
).add(
	"pool.read.events",
	"pool.update.team.add",
	"pool.update.team.remove",
	"pool.update.constraints.set",
	"pool.read.constraints",
	"pool.update.logs",
	"pool.delete",
).add(
	"debug",
).add(
	"healing.read",
).addWithCtx(
	"healing", []permTypes.ContextType{permTypes.CtxPool},
).add(
	"healing.read",
	"healing.update",
	"healing.delete",
).addWithCtx(
	"nodecontainer", []permTypes.ContextType{permTypes.CtxPool},
).add(
	"nodecontainer.create",
	"nodecontainer.read",
	"nodecontainer.update",
	"nodecontainer.update.upgrade",
	"nodecontainer.delete",
).add(
	"install.manage",
).add(
	"event-block.read",
	"event-block.read.events",
	"event-block.add",
	"event-block.remove",
).add(
	"cluster.admin",
	"cluster.read.events",
	"cluster.create",
	"cluster.update",
	"cluster.delete",
).addWithCtx(
	"volume", []permTypes.ContextType{permTypes.CtxVolume, permTypes.CtxTeam, permTypes.CtxPool},
).addWithCtx(
	"volume.create", []permTypes.ContextType{permTypes.CtxTeam, permTypes.CtxPool},
).add(
	"volume.read.events",
	"volume.update.bind",
	"volume.update.unbind",
	"volume.delete",
).addWithCtx(
	"webhook", []permTypes.ContextType{permTypes.CtxTeam},
).add(
	"webhook.read",
	"webhook.read.events",
	"webhook.create",
	"webhook.update",
	"webhook.delete",
).addWithCtx(
	"router", []permTypes.ContextType{permTypes.CtxRouter},
).addWithCtx(
	"router.create", []permTypes.ContextType{},
).add(
	"router.read.events",
	"router.update",
	"router.delete",
)
