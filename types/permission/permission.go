// Copyright 2018 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

type ContextType string

type PermissionContext struct {
	CtxType ContextType
	Value   string
}
