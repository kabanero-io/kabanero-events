/*
Copyright 2020 IBM Corporation

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
		if klog.V(6) {
			klog.Infof("Creating %s provider '%s'", provider.ProviderType, provider.Name)
		}

		var newProvider Provider
		switch provider.ProviderType {
		case "nats":
			newProvider, err = newNATSProvider(provider)
		case "rest":
			newProvider, err = newRESTProvider(provider)
		default:
			return nil, fmt.Errorf("provider '%s' for '%s' is not recognized", provider.ProviderType, provider.Name)
		}

		/* Error from trying to create new provider */
		if err != nil {
			return nil, fmt.Errorf("unable to create %s provider '%s': %v", provider.ProviderType, provider.Name, err)
		}

		err = s.Register(provider.Name, newProvider)
		if err != nil {
			return nil, fmt.Errorf("unable to register %s provider '%s': %v", provider.ProviderType, provider.Name, err)
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
