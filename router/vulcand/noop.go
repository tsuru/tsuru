// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vulcand

// Using the vulcand router is not allowed on windows for now. This file ensures
// that the package is built as a noop when GOOS=windows.
