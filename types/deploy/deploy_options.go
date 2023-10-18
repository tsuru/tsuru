package deploy

import (
	"io"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/event"
)

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

type DeployOptions struct {
	App              *app.App
	Commit           string
	BuildTag         string
	ArchiveURL       string
	Dockerfile       string
	FileSize         int64
	File             io.ReadCloser `bson:"-"`
	OutputStream     io.Writer     `bson:"-"`
	User             string
	Image            string
	Origin           string
	Event            *event.Event `bson:"-"`
	Kind             DeployKind
	Message          string
	Rollback         bool
	Build            bool
	NewVersion       bool
	OverrideVersions bool
}

func (o *DeployOptions) GetOrigin() string {
	if o.Origin != "" {
		return o.Origin
	}
	if o.Commit != "" {
		return "git"
	}
	return ""
}

func (o *DeployOptions) GetKind() (kind DeployKind) {
	if o.Kind != "" {
		return o.Kind
	}

	defer func() { o.Kind = kind }()

	if o.Dockerfile != "" {
		return DeployDockerfile
	}

	if o.Rollback {
		return DeployRollback
	}

	if o.Image != "" {
		return DeployImage
	}

	if o.File != nil {
		if o.Build {
			return DeployUploadBuild
		}
		return DeployUpload
	}

	if o.Commit != "" {
		return DeployGit
	}

	if o.ArchiveURL != "" {
		return DeployArchiveURL
	}

	return DeployKind("")
}
