/*
Copyright The CBI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BuildJob is a specification for a BuildJob resource
type BuildJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BuildJobSpec   `json:"spec"`
	Status BuildJobStatus `json:"status"`
}

// BuildJobSpec is the spec for a BuildJob resource
type BuildJobSpec struct {
	// Registry specifies the registry.
	Registry Registry `json:"registry"`
	// Language specifies the language.
	Language Language `json:"language"`
	// Context specifies the context.
	Context Context `json:"context"`
	// PluginSelector specifies additional hints for selecting the plugin
	// using the plugin labels.
	// e.g. `plugin.name = docker`.
	//
	// Controller implementations MUST have logic for selecting the default
	// plugin using Language and Context.
	// So, usually users don't need to set PluginSelector explicitly, especially
	// `language.*` labels and `context.*` labels.
	//
	// When PluginSelector is specified, Controller SHOULD select a plugin
	// that satisfies both its default logic and PluginSelector.
	//
	// +optional
	PluginSelector string `json:"pluginSelector"`
}

// Registry specifies the registry.
type Registry struct {
	// Target is used for pushing the artifact to the registry.
	// Most plugin implementations would require non-empty Target string,
	// even when Push is set to false.
	// +optional
	// e.g. `example.com:foo/bar:latest`
	Target string `json:"target"`
	// Push pushes the image.
	// Can be set to false, especially for testing purposes.
	// +optional
	Push bool `json:"push"`
	// SecretRef used for pushing and pulling.
	// +optional
	SecretRef corev1.LocalObjectReference `json:"secretRef"`
}

// Language specifies the language.
type Language struct {
	Kind string `json:"kind"`
}

const (
	// LanguageKindDockerfile stands for Dockerfile.
	// When BuildJob.Language.Kind is set to LanguageKindDockerfile, the controller
	// MUST add "language.dockerfile" to its default plugin selector logic.
	LanguageKindDockerfile = "Dockerfile"
)

// Context specifies the context.
type Context struct {
	Kind         string                      `json:"kind"`
	Git          Git                         `json:"git"`
	ConfigMapRef corev1.LocalObjectReference `json:"configMapRef"`
	HTTP         HTTP                        `json:"http"`
	Rclone       Rclone                      `json:"rclone"`
}

const (
	// ContextKindGit stands for Git context.
	// When BuildJob.Context.Kind is set to ContextKindGit, the controller
	// MUST add "context.git" to its default plugin selector logic.
	ContextKindGit = "Git"

	// ContextKindConfigMap stands for ConfigMap context.
	// When BuildJob.Context.Kind is set to ContextKindConfigMap, the controller
	// MUST add "context.configmap" to its default plugin selector logic.
	ContextKindConfigMap = "ConfigMap"

	// ContextKindHTTP stands for HTTP(S) context.
	// When BuildJob.Context.Kind is set to ContextKindHTTP, the controller
	// MUST add "context.http" to its default plugin selector logic.
	ContextKindHTTP = "HTTP"

	// ContextKindRclone stands for Rclone context.
	// When BuildJob.Context.Kind is set to ContextKindHTTP, the controller
	// MUST add "context.rclone" to its default plugin selector logic.
	ContextKindRclone = "Rclone"
)

// Git
type Git struct {
	// URL is defined in `git-clone(1)`.
	// e.g.
	// - ssh://[user@]host.xz[:port]/path/to/repo.git/
	// - [user@]host.xz:path/to/repo.git/
	//
	// Implementor's Note: url can be also like `file://`,
	// although not useful for CBI.
	URL string `json:"url"`
	// Revision such as commit, branch, or tag.
	// +optional
	Revision string `json:"revision"`
	// SubPath within the repo.
	// +optinal
	SubPath string `json:"subPath"`
	// SSHSecretRef contains the contents of ~/.ssh.
	// +optional
	SSHSecretRef corev1.LocalObjectReference `json:"sshSecretRef"`
}

// HTTP
type HTTP struct {
	// URL for a tar archive.
	// URL MUST be http:// or https:// .
	// Implementations SHOULD accept tar+gz.
	URL string `json:"url"`
	// TODO: add mediatype
	// TODO: add TLS stuff
	//
	// SubPath within the archive.
	// +optinal
	SubPath string `json:"subPath"`
}

// Rclone
type Rclone struct {
	Remote string
	Path   string
	// SecretRef contains the contents of ~/.config/rclone.
	SecretRef corev1.LocalObjectReference `json:"secretRef"`
	// SSHSecretRef contains the contents of ~/.ssh.OLD.
	// Only required for SFTP remote.
	// +optional
	SSHSecretRef corev1.LocalObjectReference `json:"sshSecretRef"`
}

// BuildJobStatus is the status for a BuildJob resource
type BuildJobStatus struct {
	Job string `json:"job"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BuildJobList is a list of BuildJob resources
type BuildJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []BuildJob `json:"items"`
}
