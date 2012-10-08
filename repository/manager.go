// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

// manager interface contains methods that a git manager should
// provide to be used within the agent.
type manager interface {
	addProject(group, project string) error
	removeProject(group, project string) error
	addGroup(group string) error
	removeGroup(group string) error
	hasGroup(group string) bool
	addMember(group, member string) error
	removeMember(group, member string) error
	buildAndStoreKeyFile(member, key string) (string, error)
	deleteKeyFile(filename string) error
	commit(message string) error
}
