package messages

import (
	"fmt"
	"k8s.io/klog"
)

// Service contains the event definition and registered providers.
type Service struct {
	eventDefinition *EventDefinition
	providers       map[string]Provider
}

// NewService initializes the message providers, and event sources and destinations.
func NewService(fileName string) (*Service, error) {
	if klog.V(5) {
		klog.Info("Initializing message service...")
		defer klog.Info("Done initializing message service")
	}

	ed, err := readEventDefinition(fileName)
	if err != nil {
		return nil, err
	}

	s := &Service{
		ed,
		make(map[string]Provider),
	}

	// Create the messaging providers
	for _, provider := range ed.Providers {
		switch provider.ProviderType {
		case "nats":
			if klog.V(6) {
				klog.Infof("Creating NATS provider '%s'", provider.Name)
			}
			natsProvider, err := newNATSProvider(provider)
			if err != nil {
				klog.Warning(err)
			}
			err = s.Register(provider.Name, natsProvider)
			if err != nil {
				klog.Warning(err)
			}
		case "rest":
			if klog.V(6) {
				klog.Infof("Creating REST provider '%s'", provider.Name)
			}
			restProvider, err := newRESTProvider(provider)
			if err != nil {
				klog.Warning(err)
			}
			err = s.Register(provider.Name, restProvider)
			if err != nil {
				klog.Warning(err)
			}
		case "kafka":
			klog.Warning("Kafka provider is not yet implemented.")
		default:
			klog.Warningf("Provider '%s' is not recognized.", provider.ProviderType)
		}
	}

	return s, nil
}

// Send a message to the destination with the name `dest`.
func (s *Service) Send(dest string, body []byte, other interface{}) error {
	node := s.GetNode(dest)
	if node == nil {
		return fmt.Errorf("unable find an event node with the name '%s'", dest)
	}

	provider := s.GetProvider(node.ProviderRef)
	if provider == nil {
		return fmt.Errorf("unable to find provider with name '%s", node.ProviderRef)
	}

	return provider.Send(node, body, other)
}

// GetProvider returns the provider with the name `name`.
func (s *Service) GetProvider(name string) Provider {
	return s.providers[name]
}

// GetNode returns the node with the hane `name`.
func (s *Service) GetNode(name string) *EventNode {
	for _, node := range s.eventDefinition.EventDestinations {
		if node.Name == name {
			return node
		}
	}
	return nil
}

// GetEventDefinition returns the structure for eventDefinitions.yaml.
func (s *Service) GetEventDefinition() *EventDefinition {
	return s.GetEventDefinition()
}

// Register a new provider.
func (s *Service) Register(name string, mp Provider) error {
	s.providers[name] = mp
	return nil
}
