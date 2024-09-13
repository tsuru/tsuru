package kubernetes

const (
	// AnnotationServiceAccountAppAnnotations can be used to add custom
	// annotations to an app service account, its value must be a serialized
	// json object that can be parsed as a map[string]string.
	AnnotationServiceAccountAppAnnotations = "app.tsuru.io/service-account-annotations"

	// AnnotationServiceAccountJobAnnotations can be used to add custom
	// annotations to a job service account, its value must be a serialized
	// json object that can be parsed as a map[string]string.
	AnnotationServiceAccountJobAnnotations = "job.tsuru.io/service-account-annotations"

	// ResourceMetadaPrefix is used to define an annotation or label for a subresource of the deployment
	// i.e: "app.tsuru.io/k8s-<resource-type>"
	// Example, setting a service annotation: 'app.tsuru.io/k8s-service={"label1": "value"}'
	ResourceMetadataPrefix = "app.tsuru.io/k8s-"

	// AnnotationEnableVPA is used to enable the creation of a recommendation
	// only VPA for the application. Its value must be a boolean.
	AnnotationEnableVPA = "app.tsuru.io/enable-vpa"

	// AnnotationKEDAPausedReplicas is used to pause the scaling of an app using KEDA scaling
	// Introduced to avoid scaling up the app when the user requested an app to be stopped
	AnnotationKEDAPausedReplicas = "autoscaling.keda.sh/paused-replicas"
)
