package push

import "testing"

func TestGenericPayloadContainsNoMessageContent(t *testing.T) {
	payload := GenericPayload()
	forbidden := []string{"message", "text", "body", "sender", "conversation"}
	for _, key := range forbidden {
		if _, ok := payload[key]; ok {
			t.Fatalf("generic push payload leaks %s", key)
		}
	}
	if payload["event"] != "new_encrypted_event_available" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}
