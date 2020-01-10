package messages

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"k8s.io/klog"
	"time"
)

// ReceiverFunc is called when an event is received from an event source.
type ReceiverFunc func([]byte)

// MessageProvider must be implemented for whichever messaging provider to be supported.
type MessageProvider interface {
	// Send a new message to an eventDestination.
	// The first parameter is message body. The second parameter is optional header or context
	Send(*EventNode, []byte, interface{}) error

	// Subscribe to events from an eventSource.
	Subscribe(*EventNode) error
	// Receive a message from an eventSource. The timeout can be configured by setting the timeout (in seconds) on the messageProvider.
	Receive(*EventNode) ([]byte, error)
	// Listen for events on an eventSource and calls the specified ReceiverFunc on the event payload.
	ListenAndServe(*EventNode, ReceiverFunc)
}

// EventDefinition contains providers, event sources, and event destinations.
type EventDefinition struct {
	MessageProviders  []*MessageProviderDefinition `yaml:"messageProviders,omitempty"`
	EventDestinations []*EventNode                 `yaml:"eventDestinations,omitempty"`
}

// MessageProviderDefinition describes a message provider and its URLs.
type MessageProviderDefinition struct {
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

var (
	eventProviders   *EventDefinition
	messageProviders map[string]MessageProvider
)

// InitializeEventProviders initializes the message providers and event sources and destinations.
func InitializeEventProviders(fileName string) (*EventDefinition, error) {
	if klog.V(5) {
		klog.Info("Initializing event providers...")
	}
	messageProviders = make(map[string]MessageProvider)
	ed, err := readEventDefinition(fileName)
	if err != nil {
		return nil, err
	}

	// Create the messaging providers
	for _, provider := range ed.MessageProviders {
		switch provider.ProviderType {
		case "nats":
			if klog.V(6) {
				klog.Infof("Creating NATS provider '%s'", provider.Name)
			}
			natsProvider, err := newNATSProvider(provider)
			if err != nil {
				klog.Warning(err)
			}
			err = RegisterProvider(provider.Name, natsProvider)
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
			err = RegisterProvider(provider.Name, restProvider)
			if err != nil {
				klog.Warning(err)
			}
		case "kafka":
			klog.Warning("Kafka provider is not yet implemented.")
		default:
			klog.Warningf("Provider '%s' is not recognized.", provider.ProviderType)
		}
	}
	eventProviders = ed
	return eventProviders, nil
}

// GetEventProviders returns a list of event providers.
func GetEventProviders() *EventDefinition {
	return eventProviders
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

// GetMessageProvider returns the MessageProvider implementation specified by name.
func (ed *EventDefinition) GetMessageProvider(name string) MessageProvider {
	return messageProviders[name]
}

// GetEventDestination returns the eventDestination specified by name.
func (ed *EventDefinition) GetEventDestination(name string) *EventNode {
	for _, node := range ed.EventDestinations {
		if node.Name == name {
			return node
		}
	}
	return nil
}

// RegisterProvider should be called to register a new provider.
func RegisterProvider(name string, mp MessageProvider) error {
	messageProviders[name] = mp
	return nil
}
