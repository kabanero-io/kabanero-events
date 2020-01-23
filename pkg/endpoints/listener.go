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

package endpoints

import (
	"encoding/json"
	"github.com/kabanero-io/kabanero-events/pkg/messages"
	"io/ioutil"
	"k8s.io/klog"
	"net/http"
	"os"
)

const (
	// HEADER Message key containing request headers
	HEADER = "header"
	// BODY Message key containing request payload
	BODY = "body"
	// WEBHOOKDESTINATION GitHub event destination
	WEBHOOKDESTINATION = "github"
)

/* Event listener */
func listenerHandler(messageService *messages.Service) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {

		header := req.Header
		klog.Infof("Received request. Header: %v", header)

		var body = req.Body

		defer body.Close()
		bytes, err := ioutil.ReadAll(body)
		if err != nil {
			klog.Errorf("Webhook listener can not read body. Error: %v", err)
		} else {
			klog.Infof("Webhook listener received body: %v", string(bytes))
		}

		var bodyMap map[string]interface{}
		err = json.Unmarshal(bytes, &bodyMap)
		if err != nil {
			klog.Errorf("Unable to unmarshal json body: %v", err)
			return
		}

		message := make(map[string]interface{})
		message[HEADER] = map[string][]string(header)
		message[BODY] = bodyMap

		bytes, err = json.Marshal(message)
		if err != nil {
			klog.Errorf("Unable to marshall as JSON: %v, type %T", message, message)
			return
		}

		err = messageService.Send(WEBHOOKDESTINATION, bytes, nil)
		if err != nil {
			klog.Errorf("Unable to send event. Error: %v", err)
			return
		}

		writer.WriteHeader(http.StatusAccepted)
	}
}

// NewListener creates a new event listener on port 9080
func NewListener(messageService *messages.Service) error {
	klog.Infof("Starting listener on port 9080")
	http.HandleFunc("/webhook", listenerHandler(messageService))
	err := http.ListenAndServe(":9080", nil)
	return err
}

// NewListenerTLS creates a new TLS event listener on port 9443
func NewListenerTLS(messageService *messages.Service, tlsCertPath, tlsKeyPath string) error {
	klog.Infof("Starting TLS listener on port 9443")
	if _, err := os.Stat(tlsCertPath); os.IsNotExist(err) {
		klog.Fatalf("TLS certificate '%s' not found: %v", tlsCertPath, err)
		return err
	}

	if _, err := os.Stat(tlsKeyPath); os.IsNotExist(err) {
		klog.Fatalf("TLS private key '%s' not found: %v", tlsKeyPath, err)
		return err
	}

	http.HandleFunc("/webhook", listenerHandler(messageService))
	err := http.ListenAndServeTLS(":9443", tlsCertPath, tlsKeyPath, nil)
	return err
}
