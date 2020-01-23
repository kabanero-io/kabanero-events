package endpoints

import (
	"github.com/kabanero-io/kabanero-events/pkg/messages"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// Environment stores clients and such that will need to be shared.
type Environment struct {
	MessageService *messages.Service
	KubeClient     *kubernetes.Clientset
	DynamicClient  dynamic.Interface
}
