// AUTOMATICALLY GENERATED FILE - DO NOT EDIT!
// Please run 'go generate' to update this file.
//
// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

var (
	PermAll                         = PermissionRegistry.get("")
	PermApp                         = PermissionRegistry.get("app")
	PermAppCreate                   = PermissionRegistry.get("app.create")
	PermAppDelete                   = PermissionRegistry.get("app.delete")
	PermAppDeploy                   = PermissionRegistry.get("app.deploy")
	PermAppRead                     = PermissionRegistry.get("app.read")
	PermAppUpdate                   = PermissionRegistry.get("app.update")
	PermAppUpdateEnv                = PermissionRegistry.get("app.update.env")
	PermAppUpdateEnvSet             = PermissionRegistry.get("app.update.env.set")
	PermAppUpdateEnvUnset           = PermissionRegistry.get("app.update.env.unset")
	PermAppUpdateRestart            = PermissionRegistry.get("app.update.restart")
	PermIaas                        = PermissionRegistry.get("iaas")
	PermIaasRead                    = PermissionRegistry.get("iaas.read")
	PermNode                        = PermissionRegistry.get("node")
	PermNodeCreate                  = PermissionRegistry.get("node.create")
	PermNodeDelete                  = PermissionRegistry.get("node.delete")
	PermNodeRead                    = PermissionRegistry.get("node.read")
	PermNodeUpdate                  = PermissionRegistry.get("node.update")
	PermServiceInstance             = PermissionRegistry.get("service-instance")
	PermServiceInstanceCreate       = PermissionRegistry.get("service-instance.create")
	PermServiceInstanceDelete       = PermissionRegistry.get("service-instance.delete")
	PermServiceInstanceRead         = PermissionRegistry.get("service-instance.read")
	PermServiceInstanceUpdate       = PermissionRegistry.get("service-instance.update")
	PermServiceInstanceUpdateBind   = PermissionRegistry.get("service-instance.update.bind")
	PermServiceInstanceUpdateGrant  = PermissionRegistry.get("service-instance.update.grant")
	PermServiceInstanceUpdateRevoke = PermissionRegistry.get("service-instance.update.revoke")
	PermServiceInstanceUpdateUnbind = PermissionRegistry.get("service-instance.update.unbind")
	PermTeam                        = PermissionRegistry.get("team")
	PermTeamCreate                  = PermissionRegistry.get("team.create")
	PermTeamDelete                  = PermissionRegistry.get("team.delete")
	PermTeamUpdate                  = PermissionRegistry.get("team.update")
	PermTeamUpdateAddMember         = PermissionRegistry.get("team.update.add-member")
	PermTeamUpdateRemoveMember      = PermissionRegistry.get("team.update.remove-member")
	PermUser                        = PermissionRegistry.get("user")
	PermUserCreate                  = PermissionRegistry.get("user.create")
	PermUserDelete                  = PermissionRegistry.get("user.delete")
	PermUserList                    = PermissionRegistry.get("user.list")
	PermUserUpdate                  = PermissionRegistry.get("user.update")
)
