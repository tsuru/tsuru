// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package container

type AppLocker interface {
	Lock(appName string) bool
	Unlock(appName string)
}
