// Copyright 2023 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package provision

type DeployKind string

const (
	DeployArchiveURL   DeployKind = "archive-url"
	DeployGit          DeployKind = "git"
	DeployImage        DeployKind = "image"
	DeployBuildedImage DeployKind = "imagebuild"
	DeployRollback     DeployKind = "rollback"
	DeployUpload       DeployKind = "upload"
	DeployUploadBuild  DeployKind = "uploadbuild"
	DeployRebuild      DeployKind = "rebuild"
	DeployDockerfile   DeployKind = "dockerfile"
)
