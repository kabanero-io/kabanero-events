package main

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v2"
	"sync"
	"testing"
	"time"
)

/*
 * TestReadEventProviders tests that event providers can be unmarshaled.
 */
func TestReadEventProviders(t *testing.T) {
	eventProviders, err := initializeEventProviders("test_data/providers0/eventDefinitions.yaml")

	if err != nil {
		t.Fatal(err)
	}

	_, err = yaml.Marshal(eventProviders)
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

	eventProviders, err := initializeEventProviders("test_data/providers0/eventDefinitions.yaml")

	if err != nil {
		t.Fatal(err)
	}

	b, err := yaml.Marshal(eventProviders)
	if err != nil {
		t.Fatal(err)
	}
	payload := string(b)
	t.Logf("Processed eventDefinitions.yaml:\n%s", payload)

	// Start listening on all eventSources
	t.Log("Creating subscriptions for event sources")

	/*
		for _, eventSource := range eventProviders.EventSources {
			provider := eventProviders.GetMessageProvider(eventSource.ProviderRef)
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
	for _, node := range eventProviders.EventDestinations {
		msg, err := json.Marshal(map[string]string{
			"msg": fmt.Sprintf("Hello %s from %s...", node.Name, node.ProviderRef),
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Sending message to event destination '%s': %s", node.Name, msg)
		provider := eventProviders.GetMessageProvider(node.ProviderRef)
		if provider == nil {
			t.Fatalf("unable to find provider referenced by event destination '%s'", node.Name)
		}
		wg.Add(1)
		go func() {
			provider.Send(node, []byte(msg), nil)
			wg.Done()
		}()
	}
	wg.Wait()
}

/*
 * TestProviderSendAndReceive is an example on sending messages to an eventSource and receiving them on an eventDestination using Subscribe/Receive.
 */
func TestProviderSendAndReceive(t *testing.T) {
	// Comment out following line to run the example
	t.SkipNow()

	eventProviders, err := initializeEventProviders("test_data/providers0/eventDefinitions.yaml")

	if err != nil {
		t.Fatal(err)
	}

	b, err := yaml.Marshal(eventProviders)
	if err != nil {
		t.Fatal(err)
	}
	payload := string(b)
	t.Logf("Processed eventDefinitions.yaml:\n%s", payload)

	// Start listening on all eventSources
	t.Log("Creating subscriptions for event sources")

	/*
		for _, eventSource := range eventProviders.EventSources {
			provider := eventProviders.GetMessageProvider(eventSource.ProviderRef)
			t.Logf("Subscribing to event source '%s'", eventSource.ProviderRef)
			err := provider.Subscribe(eventSource)
			if err != nil {
				t.Fatalf("unable to subscribe to eventSource '%s'", eventSource.Name)
			}
		}
	*/

	var wg sync.WaitGroup

	// Send the messages to the eventSources
	for _, node := range eventProviders.EventDestinations {
		msg, err := json.Marshal(map[string]string{
			"msg": fmt.Sprintf("Hello %s from %s...", node.Name, node.ProviderRef),
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Sending message to event destination '%s': %s", node.Name, msg)
		provider := eventProviders.GetMessageProvider(node.ProviderRef)
		if provider == nil {
			t.Fatalf("unable to find provider referenced by event destination '%s'", node.Name)
		}
		wg.Add(1)
		go func() {
			provider.Send(node, []byte(msg), nil)
			wg.Done()
		}()
	}
	wg.Wait()

	// And receive them here
	/*
		numMessagesExpected := 2
		for _, eventSource := range eventProviders.EventSources {
			provider := eventProviders.GetMessageProvider(eventSource.ProviderRef)

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
