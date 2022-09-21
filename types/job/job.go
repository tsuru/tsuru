// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

type Job interface {
	GetName() string
	GetPool() string
	GetTeamOwner() string
	GetTeamsName() []string
	GetExecutions() []uint
}
