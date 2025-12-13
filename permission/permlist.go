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
	"app.update.unit.kill",
	"app.update.unit.autoscale.add",
	"app.update.unit.autoscale.swap",
	"app.update.unit.autoscale.remove",
	"app.update.env.set",
	"app.update.env.unset",
	"app.update.restart",
	"app.update.start",
	"app.update.stop",
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
	"app.update.processes",
	"app.update.unbind",
	"app.update.unbind-volume",
	"app.update.certificate.set",
	"app.update.certificate.unset",
	"app.update.deploy.rollback",
	"app.update.router.add",
	"app.update.router.update",
	"app.update.router.remove",
	"app.update.routable",
	"app.update.metadata",
	"app.deploy",
	"app.deploy.archive-url",
	"app.deploy.build",
	"app.deploy.git",
	"app.deploy.image",
	"app.deploy.rollback",
	"app.deploy.upload",
	"app.deploy.dockerfile",
	"app.read",
	"app.read.deploy",
	"app.read.router",
	"app.read.env",
	"app.read.events",
	"app.read.log",
	"app.read.certificate",
	"app.read.info",
	"app.delete",
	"app.run",
	"app.run.shell",
	"app.admin.routes",
	"app.admin.quota",
	"app.build",
).addWithCtx(
	"certissuer", []permTypes.ContextType{permTypes.CtxApp, permTypes.CtxTeam, permTypes.CtxPool},
).add(
	"certissuer.set",
	"certissuer.unset",
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
	"team.read.quota",
	"team.update.quota",
).addWithCtx(
	"user", []permTypes.ContextType{permTypes.CtxUser},
).addWithCtx(
	"user.create", []permTypes.ContextType{},
).add(
	"user.delete",
	"user.read.events",
	"user.read.quota",
	"user.update.quota",
	"user.update.password",
	"user.update.reset",
).addWithCtx(
	"apikey", []permTypes.ContextType{permTypes.CtxUser},
).add(
	"apikey.read",
	"apikey.update",
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
	"platform.update.events",
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
	"pool.delete",
).add(
	"debug",
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
).addWithCtx(
	"job", []permTypes.ContextType{permTypes.CtxTeam, permTypes.CtxPool, permTypes.CtxJob},
).addWithCtx(
	"job.create", []permTypes.ContextType{permTypes.CtxTeam},
).add(
	"job.update",
).add(
	"job.run",
).add(
	"job.read",
).add(
	"job.delete",
).add(
	"job.read.events",
	"job.update.events",
).add(
	"job.read.logs",
).add(
	"job.trigger",
).add(
	"job.unit.kill",
).add(
	"job.deploy",
)
