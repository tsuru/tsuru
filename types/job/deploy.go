// Copyright 2024 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package job

import (
	"io"

	provisionTypes "github.com/tsuru/tsuru/types/provision"
)

type DeployOptions struct {
	JobName    string
	Kind       provisionTypes.DeployKind `json:"kind"`
	Image      string                    `json:"image"`
	FileSize   int64                     `json:"fileSize"`
	File       io.ReadCloser             `bson:"-"`
	Dockerfile string                    `json:"dockerfile"`
	Message    string
	User       string
}

func (deployOptions *DeployOptions) GetKind() (kind provisionTypes.DeployKind) {
	if deployOptions.Kind != "" {
		return deployOptions.Kind
	}

	defer func() { deployOptions.Kind = kind }()

	if deployOptions.Dockerfile != "" {
		return provisionTypes.DeployDockerfile
	}

	if deployOptions.Image != "" {
		return provisionTypes.DeployImage
	}

	return provisionTypes.DeployKind("")
}
