package application

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCurrentSessionIdentityTransformerRewritesOnlyTypedProjectIdentity(t *testing.T) {
	intent, err := json.Marshal(mutationIntentEvent{Prepared: PreparedTransition{
		Version: PreparedTransitionVersion,
		Target:  MutationTarget{Project: "local", Branch: "main"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	untyped := json.RawMessage(`{"project":"local"}`)
	metadata := SessionMetadata{CodecVersion: SessionCodecVersion, ID: "session", Subject: "local", Project: "local"}
	result, err := (CurrentSessionIdentityTransformer{}).RewriteProjectIdentity("github.com/org/repo", SessionIdentityEnvelope{
		Metadata: &metadata,
		Events: []StoredEvent{
			{CodecVersion: SessionCodecVersion, Code: eventMutationIntent, Payload: intent},
			{CodecVersion: SessionCodecVersion, Code: "user_payload", Payload: untyped},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metadata.Project != "github.com/org/repo" {
		t.Fatalf("metadata project = %q", result.Metadata.Project)
	}
	if !bytes.Contains(result.Events[0].Payload, []byte(`"project":"github.com/org/repo"`)) {
		t.Fatalf("mutation target project was not rewritten: %s", result.Events[0].Payload)
	}
	if !bytes.Equal(result.Events[1].Payload, untyped) {
		t.Fatalf("untyped payload was rewritten: %s", result.Events[1].Payload)
	}
}
