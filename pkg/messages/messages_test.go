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

package messages_test

import (
	"encoding/json"
	"fmt"
	"github.com/kabanero-io/kabanero-events/pkg/messages"
	"sync"
	"testing"
	"time"
)

const testDataDir = "../../test_data"

func getEventDefinitions(provider string) string {
	return fmt.Sprintf("%s/%s/eventDefinitions.yaml", testDataDir, provider)
}

/*
 * TestReadEventProviders tests that event providers can be unmarshalled.
 */
func TestReadEventProviders(t *testing.T) {
	// Comment out following line to run the example
	t.SkipNow()

	_, err := messages.NewService(getEventDefinitions("providers0"))
	if err != nil {
		t.Fatal(err)
	}
}

/*
 * TestProviderListenAndSend is an example on setting up event listeners to receive messages sent to an eventSource.
 */
func TestProviderListenAndSend(t *testing.T) {
	// Comment out following line to run the example
	t.SkipNow()

	messageService, err := messages.NewService(getEventDefinitions("providers0"))
	eventDefinition := messageService.GetEventDefinition()

	if err != nil {
		t.Fatal(err)
	}

	// Start listening on all eventSources
	t.Log("Creating subscriptions for event sources")

	/*
		for _, eventSource := range eventDefinition.EventSources {
			provider := eventDefinition.GetMessageProvider(eventSource.ProviderRef)
			t.Logf("Subscribing to event source '%s'", eventSource.ProviderRef)
			echoMessage := func(data []byte) {
				t.Logf("Message body: %s", data)
			}
			go provider.ListenAndServe(eventSource, echoMessage)
		}
	*/

	// Give the listener a bit of time to start up before sending messages
	time.Sleep(1 * time.Second)

	var wg sync.WaitGroup
	for _, node := range eventDefinition.EventDestinations {
		msg, err := json.Marshal(map[string]string{
			"msg": fmt.Sprintf("Hello %s from %s...", node.Name, node.ProviderRef),
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Sending message to event destination '%s': %s", node.Name, msg)
		provider := messageService.GetProvider(node.ProviderRef)
		if provider == nil {
			t.Fatalf("unable to find provider referenced by event destination '%s'", node.Name)
		}
		wg.Add(1)
		go func(node *messages.EventNode) {
			err := provider.Send(node, []byte(msg), nil)
			if err != nil {
				t.Errorf("unable to send event: %v", err)
			}
			wg.Done()
		}(node)
	}
	wg.Wait()
}

/*
 * TestProviderSendAndReceive is an example on sending messages to an eventSource and receiving them on an eventDestination using Subscribe/Receive.
 */
func TestProviderSendAndReceive(t *testing.T) {
	// Comment out following line to run the example
	t.SkipNow()

	messageService, err := messages.NewService(getEventDefinitions("providers0"))
	eventDefinition := messageService.GetEventDefinition()

	if err != nil {
		t.Fatal(err)
	}

	// Start listening on all eventSources
	t.Log("Creating subscriptions for event sources")

	/*
		for _, eventSource := range eventDefinition.EventSources {
			provider := eventDefinition.GetMessageProvider(eventSource.ProviderRef)
			t.Logf("Subscribing to event source '%s'", eventSource.ProviderRef)
			err := provider.Subscribe(eventSource)
			if err != nil {
				t.Fatalf("unable to subscribe to eventSource '%s'", eventSource.Name)
			}
		}
	*/

	var wg sync.WaitGroup

	// Send the messages to the eventSources
	for _, node := range eventDefinition.EventDestinations {
		msg, err := json.Marshal(map[string]string{
			"msg": fmt.Sprintf("Hello %s from %s...", node.Name, node.ProviderRef),
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Sending message to event destination '%s': %s", node.Name, msg)
		provider := messageService.GetProvider(node.ProviderRef)
		if provider == nil {
			t.Fatalf("unable to find provider referenced by event destination '%s'", node.Name)
		}
		wg.Add(1)
		go func(node *messages.EventNode) {
			err := provider.Send(node, []byte(msg), nil)
			if err != nil {
				t.Errorf("unable to send event: %v", err)
			}
			wg.Done()
		}(node)
	}
	wg.Wait()

	// And receive them here
	/*
		numMessagesExpected := 2
		for _, eventSource := range eventDefinition.EventSources {
			provider := eventDefinition.GetMessageProvider(eventSource.ProviderRef)

			// Try maxAttempts times to receive messages before giving up
			for i := 0; i < numMessagesExpected; i++ {
				t.Logf("Waiting for message %d / %d eventSource '%s'", i + 1, numMessagesExpected, eventSource.Name)
				b, err := provider.Receive(eventSource)
				if err != nil {
					t.Fatalf("timed out waiting for a message from eventSource '%s'", eventSource.Name)
				}
				t.Logf("Received message from eventSource '%s': %s", eventSource.Name, b)
			}
		}
	*/
}
