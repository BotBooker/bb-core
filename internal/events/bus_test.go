package events

import (
	"testing"
)

func TestNewEventBus(t *testing.T) {
	tests := []struct {
		name       string
		wantNotNil bool
	}{
		{
			name:       "create event bus",
			wantNotNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewEventBus()
			if tt.wantNotNil && got == nil {
				t.Error("NewEventBus() returned nil, expected non-nil EventBus")
			}
		})
	}
}

func TestEventBus_Publish(t *testing.T) {
	b := NewEventBus()

	tests := []struct {
		name  string
		event string
	}{
		{
			name:  "publish simple event",
			event: "user.created",
		},
		{
			name:  "publish empty event",
			event: "",
		},
		{
			name:  "publish event with special characters",
			event: "order.completed:id=123&type=premium",
		},
		{
			name:  "publish very long event name",
			event: "this_is_a_very_long_event_name_that_exceeds_normal_length_limits_to_test_boundary_conditions",
		},
		{
			name:  "publish multiple word event",
			event: "payment.failed.retry.exhausted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			b.Publish(tt.event)
		})
	}
}

func TestEventBus_Subscribe(t *testing.T) {
	b := NewEventBus()

	handler := func() {}

	tests := []struct {
		name    string
		event   string
		handler func()
	}{
		{
			name:    "subscribe to simple event",
			event:   "test.event",
			handler: handler,
		},
		{
			name:    "subscribe to empty event",
			event:   "",
			handler: handler,
		},
		{
			name:    "subscribe with nil handler",
			event:   "test.event",
			handler: nil,
		},
		{
			name:    "subscribe to same event multiple times",
			event:   "duplicate.event",
			handler: handler,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			b.Subscribe(tt.event, tt.handler)
		})
	}
}

// Note: The eventBus implementation is a stub that only logs.
// It does not actually store or invoke handlers.
// These tests verify that the methods don't panic.

func TestEventBus_PublishAndSubscribe_NoPanic(t *testing.T) {
	b := NewEventBus()

	handler := func() {}

	b.Subscribe("test.event", handler)
	b.Publish("test.event")

	// Should not panic - the stub implementation doesn't call handlers
}

func TestEventBus_MultipleSubscribers_NoPanic(t *testing.T) {
	b := NewEventBus()

	for i := 0; i < 3; i++ {
		b.Subscribe("multi.event", func() {})
	}

	b.Publish("multi.event")

	// Should not panic
}

func TestEventBus_DifferentEvents_NoPanic(t *testing.T) {
	b := NewEventBus()

	b.Subscribe("event.a", func() {})
	b.Subscribe("event.b", func() {})

	b.Publish("event.a")
	b.Publish("event.b")

	// Should not panic
}

func TestEventBus_NoSubscribers(t *testing.T) {
	b := NewEventBus()

	// Should not panic when publishing to event with no subscribers
	b.Publish("unsubscribed.event")
}

func TestEventBus_MultiplePublishes(t *testing.T) {
	b := NewEventBus()

	b.Subscribe("repeat.event", func() {})

	for i := 0; i < 5; i++ {
		b.Publish("repeat.event")
	}

	// Should not panic
}
