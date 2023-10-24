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

type DeployOptions interface {
	GetKind() (kind DeployKind)
}
