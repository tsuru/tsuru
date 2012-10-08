// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package repository

type recordingManager struct {
	actions        map[string][]string
	hasGroupReturn bool
}

func newRecordingManager() *recordingManager {
	return &recordingManager{
		actions: make(map[string][]string),
	}
}

func (r *recordingManager) addProject(group, project string) error {
	r.actions["addProject"] = []string{group, project}
	return nil
}

func (r *recordingManager) removeProject(group, project string) error {
	r.actions["removeProject"] = []string{group, project}
	return nil
}

func (r *recordingManager) addGroup(group string) error {
	r.actions["addGroup"] = []string{group}
	return nil
}

func (r *recordingManager) removeGroup(group string) error {
	r.actions["removeGroup"] = []string{group}
	return nil
}

func (r *recordingManager) hasGroup(group string) bool {
	return r.hasGroupReturn
}

func (r *recordingManager) addMember(group, member string) error {
	r.actions["addMember"] = []string{group, member}
	return nil
}

func (r *recordingManager) removeMember(group, member string) error {
	r.actions["removeMember"] = []string{group, member}
	return nil
}

func (r *recordingManager) buildAndStoreKeyFile(member, key string) (string, error) {
	r.actions["build"] = []string{member, key}
	return member + ".key1.pub", nil
}

func (r *recordingManager) deleteKeyFile(filename string) error {
	r.actions["removeKey"] = []string{filename}
	return nil
}

func (r *recordingManager) commit(message string) error {
	r.actions["commit"] = []string{message}
	return nil
}
