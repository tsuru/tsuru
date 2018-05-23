package cbi_plugin_v1

var (
	// PredefinedLabelPrefixes is the set of predefined plugin label prefixes.
	// Although plugin labels are decoupled from k8s object labels,
	// we try to follow the k8s convention.
	//
	// At the moment, we don't use long prefix such as "plugin.cbi.containerbuilding.github.io".
	// However, the convention might change in v1 GA.
	PredefinedLabelPrefixes = []string{
		"plugin.",
		"language.",
		"context.",
	}
)

const (
	// LPluginName is the NON-UNIQUE name of the plugin.
	// LPluginName MUST be always present.
	//
	// Example values: "buildkit", "buildah", ...
	LPluginName = "plugin.name"
	// TODO: add LPluginVersion = "v1alpha1"?
)

const (
	// LLanguageDockerfile SHOULD be present if the plugin accepts a BuildJob
	// with Language.Kind = LanguageKindDockerfile
	//
	// Value SHOULD be empty.
	LLanguageDockerfile = "language.dockerfile"
	// TODO: add LLanguageDockerfileMultistage = "language.dockerfile.multistage"...
)

const (
	// LContextGit SHOULD be present if the plugin accepts a BuildJob
	// with Context.Kind = ContextKindGit
	//
	// Value SHOULD be empty.
	LContextGit = "context.git"

	// LContextConfigMap SHOULD be present if the plugin accepts a BuildJob
	// with Context.Kind = ContextKindConfigMap
	//
	// Value SHOULD be empty.
	LContextConfigMap = "context.configmap"

	// LContextHTTP SHOULD be present if the plugin accepts a BuildJob
	// with Context.Kind = ContextKindHTTP
	//
	// Value SHOULD be empty.
	LContextHTTP = "context.http"

	// LContextRclone SHOULD be present if the plugin accepts a BuildJob
	// with Context.Kind = ContextKindRclone
	//
	// Value SHOULD be empty.
	LContextRclone = "context.rclone"
)
