package application

import (
	"encoding/json"
	"fmt"
)

// SessionIdentityTransformerVersion is persisted in relocation manifests so
// an interrupted rewrite never resumes under changed event semantics.
const SessionIdentityTransformerVersion uint32 = 1

// SessionIdentityEnvelope is one current-format session-log line's
// application-owned identity-bearing content. Filesystem encoding remains a
// local-adapter concern.
type SessionIdentityEnvelope struct {
	Metadata *SessionMetadata
	Events   []StoredEvent
}

// SessionIdentityTransformer owns the semantic knowledge of which current
// session fields carry project identity.
type SessionIdentityTransformer interface {
	Version() uint32
	RewriteProjectIdentity(ProjectID, SessionIdentityEnvelope) (SessionIdentityEnvelope, error)
}

// CurrentSessionIdentityTransformer rewrites the current session codec.
type CurrentSessionIdentityTransformer struct{}

func (CurrentSessionIdentityTransformer) Version() uint32 {
	return SessionIdentityTransformerVersion
}

func (CurrentSessionIdentityTransformer) RewriteProjectIdentity(target ProjectID, envelope SessionIdentityEnvelope) (SessionIdentityEnvelope, error) {
	if target == "" {
		return SessionIdentityEnvelope{}, fmt.Errorf("sdd: target project identity is required")
	}
	result := SessionIdentityEnvelope{Events: append([]StoredEvent(nil), envelope.Events...)}
	if envelope.Metadata != nil {
		metadata := *envelope.Metadata
		switch metadata.Project {
		case "local":
			metadata.Project = target
		case target:
		default:
			return SessionIdentityEnvelope{}, fmt.Errorf("session %s belongs to project %s, not relocation target %s", metadata.ID, metadata.Project, target)
		}
		result.Metadata = &metadata
	}
	for index := range result.Events {
		if err := rewriteStoredEventProject(&result.Events[index], target); err != nil {
			return SessionIdentityEnvelope{}, err
		}
	}
	return result, nil
}

func rewriteStoredEventProject(event *StoredEvent, target ProjectID) error {
	var path []string
	switch event.Code {
	case eventMutationIntent:
		path = []string{"prepared", "Target", "project"}
	case eventRecoveryAttempt, eventRecoveryTerminal, eventLegacyTargetBound:
		path = []string{"target", "project"}
	default:
		return nil
	}
	rewritten, changed, err := rewriteJSONProject(event.Payload, path, target)
	if err != nil {
		return fmt.Errorf("%s: %w", event.Code, err)
	}
	if changed {
		event.Payload = rewritten
	}
	return nil
}

// rewriteJSONProject changes only the typed MutationTarget project path while
// preserving every unrelated raw field. Workflow payloads can contain
// arbitrary user values, so a recursive key-wide replacement would be unsafe.
func rewriteJSONProject(raw json.RawMessage, path []string, target ProjectID) (json.RawMessage, bool, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, false, err
	}
	field, ok := object[path[0]]
	if !ok {
		return raw, false, nil
	}
	if len(path) == 1 {
		var project ProjectID
		if err := json.Unmarshal(field, &project); err != nil {
			return nil, false, err
		}
		switch project {
		case "":
			return raw, false, nil
		case "local":
			encoded, err := json.Marshal(target)
			if err != nil {
				return nil, false, err
			}
			object[path[0]] = encoded
		case target:
			return raw, false, nil
		default:
			return nil, false, fmt.Errorf("event belongs to project %s, not relocation target %s", project, target)
		}
	} else {
		rewritten, changed, err := rewriteJSONProject(field, path[1:], target)
		if err != nil || !changed {
			return raw, false, err
		}
		object[path[0]] = rewritten
	}
	encoded, err := json.Marshal(object)
	return encoded, err == nil, err
}
