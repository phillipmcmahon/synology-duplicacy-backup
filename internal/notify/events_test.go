package notify

import "testing"

func TestKnownEventsIncludesUpdateAndRuntimeEvents(t *testing.T) {
	for _, event := range []EventID{
		EventNotificationTest,
		EventBackupFailed,
		EventHealthUnhealthy,
		EventUpdateInstallFailed,
		EventUpdateInstallSucceeded,
	} {
		if !IsKnownEvent(string(event)) {
			t.Fatalf("IsKnownEvent(%q) = false", event)
		}
	}
	if IsKnownEvent("discord_message_sent") {
		t.Fatal("unexpected custom event accepted")
	}
}
