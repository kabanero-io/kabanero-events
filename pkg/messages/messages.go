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
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"k8s.io/klog"
	"time"
)

// ReceiverFunc is called when an event is received from an event source.
type ReceiverFunc func([]byte)

// Provider must be implemented for whichever messaging provider to be supported.
type Provider interface {
	// Send a new message to an eventDestination.
	// The first parameter is message body. The second parameter is optional header or context
	Send(*EventNode, []byte, interface{}) error
	// Subscribe to eventDefinition from an eventSource.
	Subscribe(*EventNode) error
	// Receive a message from an eventSource. The timeout can be configured by setting the timeout (in seconds) on the messageProvider.
	Receive(*EventNode) ([]byte, error)
	// Listen for eventDefinition on an eventSource and calls the specified ReceiverFunc on the event payload.
	ListenAndServe(*EventNode, ReceiverFunc)
}

// EventDefinition contains providers, event sources, and event destinations.
type EventDefinition struct {
	Providers         []*ProviderDefinition `yaml:"messageProviders,omitempty"`
	EventDestinations []*EventNode          `yaml:"eventDestinations,omitempty"`
}

// ProviderDefinition describes a message provider and its URLs.
type ProviderDefinition struct {
	Name          string        `yaml:"name"`
	ProviderType  string        `yaml:"providerType"`
	URL           string        `yaml:"url"`
	Timeout       time.Duration `yaml:"timeout"`
	SkipTLSVerify bool          `yaml:"skipTLSVerify,omitempty"`
}

// EventNode represents either an event source or destination and consists of a provider reference and the topic to
// either send to or receive from.
type EventNode struct {
	Name        string `yaml:"name"`
	Topic       string `yaml:"topic"`
	ProviderRef string `yaml:"providerRef"`
}

func readEventDefinition(fileName string) (*EventDefinition, error) {
	if klog.V(5) {
		klog.Infof("Reading event providers from '%s'", fileName)
	}

	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	var ed EventDefinition
	err = yaml.Unmarshal(bytes, &ed)
	return &ed, err
}
