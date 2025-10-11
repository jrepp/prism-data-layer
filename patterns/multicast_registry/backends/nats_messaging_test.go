package backends

import (
	"context"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/test"
)

func TestNATSMessaging_PublishSubscribe(t *testing.T) {
	// Start embedded NATS server
	opts := natsserver.DefaultTestOptions
	opts.Port = -1 // Random port
	s := natsserver.RunServer(&opts)
	defer s.Shutdown()

	backend, err := NewNATSMessagingBackend([]string{s.ClientURL()})
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Subscribe to topic
	topic := "test.topic"
	msgChan, err := backend.Subscribe(ctx, topic)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Publish message
	payload := []byte("hello world")
	err = backend.Publish(ctx, topic, payload)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Receive message
	select {
	case received := <-msgChan:
		if string(received) != string(payload) {
			t.Errorf("Expected %s, got %s", payload, received)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for message")
	}
}

func TestNATSMessaging_MultiplePublish(t *testing.T) {
	opts := natsserver.DefaultTestOptions
	opts.Port = -1
	s := natsserver.RunServer(&opts)
	defer s.Shutdown()

	backend, err := NewNATSMessagingBackend([]string{s.ClientURL()})
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Subscribe
	topic := "test.multi"
	msgChan, err := backend.Subscribe(ctx, topic)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Publish 5 messages
	messageCount := 5
	for i := 0; i < messageCount; i++ {
		payload := []byte("message-" + string(rune('0'+i)))
		if err := backend.Publish(ctx, topic, payload); err != nil {
			t.Fatalf("Publish %d failed: %v", i, err)
		}
	}

	// Receive all messages
	received := 0
	timeout := time.After(2 * time.Second)
	for received < messageCount {
		select {
		case <-msgChan:
			received++
		case <-timeout:
			t.Fatalf("Timeout: received %d/%d messages", received, messageCount)
		}
	}
}

func TestNATSMessaging_Unsubscribe(t *testing.T) {
	opts := natsserver.DefaultTestOptions
	opts.Port = -1
	s := natsserver.RunServer(&opts)
	defer s.Shutdown()

	backend, err := NewNATSMessagingBackend([]string{s.ClientURL()})
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	ctx := context.Background()

	// Subscribe
	topic := "test.unsub"
	msgChan, err := backend.Subscribe(ctx, topic)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Unsubscribe
	err = backend.Unsubscribe(ctx, topic)
	if err != nil {
		t.Fatalf("Unsubscribe failed: %v", err)
	}

	// Publish after unsubscribe
	backend.Publish(ctx, topic, []byte("should not receive"))

	// Should not receive message
	select {
	case <-msgChan:
		t.Error("Received message after unsubscribe")
	case <-time.After(500 * time.Millisecond):
		// Expected: no message received
	}
}

func TestNATSMessaging_FanoutDelivery(t *testing.T) {
	opts := natsserver.DefaultTestOptions
	opts.Port = -1
	s := natsserver.RunServer(&opts)
	defer s.Shutdown()

	backend, err := NewNATSMessagingBackend([]string{s.ClientURL()})
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create 3 subscribers to same topic
	topic := "test.fanout"
	var subscribers []<-chan []byte
	for i := 0; i < 3; i++ {
		// Create separate backend for each subscriber (simulates different consumers)
		subBackend, _ := NewNATSMessagingBackend([]string{s.ClientURL()})
		defer subBackend.Close()

		msgChan, err := subBackend.Subscribe(ctx, topic)
		if err != nil {
			t.Fatalf("Subscribe %d failed: %v", i, err)
		}
		subscribers = append(subscribers, msgChan)
	}

	// Wait for subscriptions to be fully established
	time.Sleep(100 * time.Millisecond)

	// Publish one message
	payload := []byte("fanout message")
	err = backend.Publish(ctx, topic, payload)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// All 3 subscribers should receive
	for i, msgChan := range subscribers {
		select {
		case received := <-msgChan:
			if string(received) != string(payload) {
				t.Errorf("Subscriber %d: expected %s, got %s", i, payload, received)
			}
		case <-time.After(1 * time.Second):
			t.Errorf("Subscriber %d: timeout waiting for message", i)
		}
	}
}

func TestNATSMessaging_PublishWithoutSubscribers(t *testing.T) {
	opts := natsserver.DefaultTestOptions
	opts.Port = -1
	s := natsserver.RunServer(&opts)
	defer s.Shutdown()

	backend, err := NewNATSMessagingBackend([]string{s.ClientURL()})
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Publish to topic with no subscribers (should not error)
	err = backend.Publish(ctx, "no.subscribers", []byte("lost message"))
	if err != nil {
		t.Errorf("Publish without subscribers should succeed, got error: %v", err)
	}
}

func TestNATSMessaging_ConnectionStatus(t *testing.T) {
	opts := natsserver.DefaultTestOptions
	opts.Port = -1
	s := natsserver.RunServer(&opts)
	defer s.Shutdown()

	backend, err := NewNATSMessagingBackend([]string{s.ClientURL()})
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}
	defer backend.Close()

	status := backend.ConnectionStatus()
	if status != "CONNECTED" {
		t.Errorf("Expected CONNECTED status, got %s", status)
	}

	// Close and check status
	backend.Close()
	status = backend.ConnectionStatus()
	if status != "CLOSED" {
		t.Errorf("Expected CLOSED status after close, got %s", status)
	}
}
