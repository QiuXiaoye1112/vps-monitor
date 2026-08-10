package realtime

import (
	"testing"
	"time"
)

func TestHubPublishesEventsWithoutBlocking(t *testing.T) {
	hub := NewHub()
	events, unsubscribe := hub.Subscribe()
	defer unsubscribe()

	want := Event{Kind: KindPing, UUID: "node-1", TaskID: 7, Time: "2026-08-10T00:00:10Z", Value: 42}
	hub.Publish(want)

	select {
	case got := <-events:
		if got.Kind != want.Kind || got.UUID != want.UUID || got.TaskID != want.TaskID || got.Time != want.Time || got.Value != want.Value {
			t.Fatalf("event = %+v, want %+v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestHubDropsEventsForSlowSubscribersInsteadOfBlocking(t *testing.T) {
	hub := NewHub()
	_, unsubscribe := hub.Subscribe()
	defer unsubscribe()

	for i := 0; i < 256; i++ {
		hub.Publish(Event{Kind: KindStatus, UUID: "node-1"})
	}
}
