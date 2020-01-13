package resources

import (
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// Resource contains resources like k8s clients that we may need across the application.
type Resource struct {
	KubeClient    *kubernetes.Clientset
	DynamicClient dynamic.Interface
}
