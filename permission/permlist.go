// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

//go:generate go run ./generator/main.go -o permitems.go

var PermissionRecord = (&recorder{}).addWithContextCB("app", []contextType{CtxApp, CtxTeam, CtxPool}, func(r *recorder) {
	r.addWithContext("create", []contextType{CtxTeam, CtxPool})
	r.add("read", "delete", "deploy")
	r.addCB("update", func(r *recorder) {
		r.addCB("env", func(r *recorder) {
			r.add("set", "unset")
		})
		r.add("restart")
	})
}).addWithContextCB("node", []contextType{CtxPool}, func(r *recorder) {
	r.add("create", "read", "update", "delete")
}).addWithContextCB("iaas", []contextType{CtxIaaS}, func(r *recorder) {
	r.add("create", "read", "update", "delete")
}).addWithContextCB("team", []contextType{CtxTeam}, func(r *recorder) {
	r.addWithContext("create", []contextType{})
	r.add("delete")
	r.addCB("update", func(r *recorder) {
		r.add("add-member", "remove-member")
	})
}).addCB("user", func(r *recorder) {
	r.add("create", "delete", "list", "update")
}).addWithContextCB("service-instance", []contextType{CtxServiceInstance, CtxTeam}, func(r *recorder) {
	r.addWithContext("create", []contextType{CtxTeam})
	r.addCB("update", func(r *recorder) {
		r.add("bind", "unbind", "grant", "revoke")
	})
	r.add("delete", "read")
})
