// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tag

import tagTypes "github.com/tsuru/tsuru/types/tag"

func TagService() (tagTypes.TagServiceClient, error) {
	// TODO: check conf and initialize correct client
	return &noopTagClient{}, nil
}
