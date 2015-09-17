// AUTOMATICALLY GENERATED FILE - DO NOT EDIT!
// Please run 'go generate' to update this file.
//
// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

var (
	PermAll                         = PermissionRecord.get("")
	PermApp                         = PermissionRecord.get("app")
	PermAppCreate                   = PermissionRecord.get("app.create")
	PermAppDelete                   = PermissionRecord.get("app.delete")
	PermAppDeploy                   = PermissionRecord.get("app.deploy")
	PermAppRead                     = PermissionRecord.get("app.read")
	PermAppUpdate                   = PermissionRecord.get("app.update")
	PermAppUpdateEnv                = PermissionRecord.get("app.update.env")
	PermAppUpdateEnvSet             = PermissionRecord.get("app.update.env.set")
	PermAppUpdateEnvUnset           = PermissionRecord.get("app.update.env.unset")
	PermAppUpdateRestart            = PermissionRecord.get("app.update.restart")
	PermIaas                        = PermissionRecord.get("iaas")
	PermIaasCreate                  = PermissionRecord.get("iaas.create")
	PermIaasDelete                  = PermissionRecord.get("iaas.delete")
	PermIaasRead                    = PermissionRecord.get("iaas.read")
	PermIaasUpdate                  = PermissionRecord.get("iaas.update")
	PermNode                        = PermissionRecord.get("node")
	PermNodeCreate                  = PermissionRecord.get("node.create")
	PermNodeDelete                  = PermissionRecord.get("node.delete")
	PermNodeRead                    = PermissionRecord.get("node.read")
	PermNodeUpdate                  = PermissionRecord.get("node.update")
	PermServiceInstance             = PermissionRecord.get("service-instance")
	PermServiceInstanceCreate       = PermissionRecord.get("service-instance.create")
	PermServiceInstanceDelete       = PermissionRecord.get("service-instance.delete")
	PermServiceInstanceRead         = PermissionRecord.get("service-instance.read")
	PermServiceInstanceUpdate       = PermissionRecord.get("service-instance.update")
	PermServiceInstanceUpdateBind   = PermissionRecord.get("service-instance.update.bind")
	PermServiceInstanceUpdateGrant  = PermissionRecord.get("service-instance.update.grant")
	PermServiceInstanceUpdateRevoke = PermissionRecord.get("service-instance.update.revoke")
	PermServiceInstanceUpdateUnbind = PermissionRecord.get("service-instance.update.unbind")
	PermTeam                        = PermissionRecord.get("team")
	PermTeamCreate                  = PermissionRecord.get("team.create")
	PermTeamDelete                  = PermissionRecord.get("team.delete")
	PermTeamUpdate                  = PermissionRecord.get("team.update")
	PermTeamUpdateAddMember         = PermissionRecord.get("team.update.add-member")
	PermTeamUpdateRemoveMember      = PermissionRecord.get("team.update.remove-member")
	PermUser                        = PermissionRecord.get("user")
	PermUserCreate                  = PermissionRecord.get("user.create")
	PermUserDelete                  = PermissionRecord.get("user.delete")
	PermUserList                    = PermissionRecord.get("user.list")
	PermUserUpdate                  = PermissionRecord.get("user.update")
)
