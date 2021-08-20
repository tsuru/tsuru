package kubernetes

const (
	// AnnotationServiceAccountAnnotations can be used to add custom
	// annotations to an app service account, its value must be a serialized
	// json object that can be parsed as a map[string]string.
	AnnotationServiceAccountAnnotations = "app.tsuru.io/service-account-annotations"

	// AnnotationEnableVPA is used to enable the creation of a recommendation
	// only VPA for the application. Its value must be a boolean.
	AnnotationEnableVPA = "app.tsuru.io/enable-vpa"
)
