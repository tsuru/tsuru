// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fix

import (
	"regexp"

	"github.com/pkg/errors"
)

var digestRegexp = regexp.MustCompile(`(?m)^Digest: (.*)$`)

func GetImageDigest(pullOutput string) (string, error) {
	match := digestRegexp.FindAllStringSubmatch(pullOutput, 1)
	if len(match) <= 0 {
		return "", errors.New("Can't get image digest")
	}
	return match[0][1], nil
}
