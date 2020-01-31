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
	"bytes"
	"crypto/tls"
	"fmt"
	"k8s.io/klog"
	"net/http"
	"time"
)

type restProvider struct {
	messageProviderDefinition *ProviderDefinition
}

func (provider *restProvider) initialize(mpd *ProviderDefinition) error {
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

	req, err := http.NewRequest("POST", provider.messageProviderDefinition.URL, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}

	if header != nil {
		headerMap, ok := header.(map[string][]string)
		if !ok {
			return fmt.Errorf("restProvider.Send: header not map[string][]string")
		}
		for key, arrayString := range headerMap {
			for _, str := range arrayString {
				req.Header.Add(key, str)
			}
		}
	}
	req.Header.Add("Content-Type", "application/json")

	tr := &http.Transport{}
	if provider.messageProviderDefinition.SkipTLSVerify {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	timeout := time.Duration(provider.messageProviderDefinition.Timeout * time.Second)
	client := &http.Client{
		Transport: tr,
		Timeout:   timeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("res_provider Send to %v failed with http status %v", provider.messageProviderDefinition.URL, resp.Status)
	}

	return nil
}

// Receive is not implemented for REST providers.
func (provider *restProvider) Receive(node *EventNode) ([]byte, error) {
	klog.Fatal("receiving on a REST provider is not supported")
	return nil, nil
}

func newRESTProvider(mpd *ProviderDefinition) (*restProvider, error) {
	provider := new(restProvider)
	if err := provider.initialize(mpd); err != nil {
		return nil, err
	}

	return provider, nil
}
