package main

import (
	"bytes"
	"k8s.io/klog"
	"net/http"
)

type restProvider struct {
	messageProviderDefinition *MessageProviderDefinition
}

func (provider *restProvider) initialize(mpd *MessageProviderDefinition) error {
	provider.messageProviderDefinition = mpd
	return nil
}

// Subscribe is not implemented for REST providers.
func (provider *restProvider) Subscribe(node *EventNode) error {
	klog.Fatal("subscribing on a REST provider is not supported")
	return nil
}

// ListenAndServe is not implemented for REST providers.
func (provider *restProvider) ListenAndServe(node *EventNode, receiver ReceiverFunc) {
	klog.Fatal("listening on a REST provider is not supported")
}

// Send a message to an eventDestination.
func (provider *restProvider) Send(node *EventNode, payload []byte, header interface{}) error {
	if klog.V(6) {
		klog.Infof("restProvider: Sending %s", string(payload))
	}
	resp, err := http.Post(provider.messageProviderDefinition.URL,
		"application/json",
		bytes.NewBuffer(payload))

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	return nil
}

// Receive is not implemented for REST providers.
func (provider *restProvider) Receive(node *EventNode) ([]byte, error) {
	klog.Fatal("receiving on a REST provider is not supported")
	return nil, nil
}

func newRESTProvider(mpd *MessageProviderDefinition) (*restProvider, error) {
	provider := new(restProvider)
	if err := provider.initialize(mpd); err != nil {
		return nil, err
	}

	return provider, nil
}
