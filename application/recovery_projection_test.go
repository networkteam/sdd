package application

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Real v1 intents extracted from the live session store; see
// testdata/recovery/README.md for the source logs and the selection rule.
const (
	strandedFixtureSession    = "s_20260714-095955-885b3c45"
	strandedFixtureMutationID = "entry-20260714-103304-s-tac-rcv"
)

// recoveryFixtureLine mirrors the store's on-disk session envelope. The test
// reads the envelope only; every projection fact under test still comes from
// replayRecovery and recoveryItem, the real seams.
type recoveryFixtureLine struct {
	Version  uint64           `json:"version"`
	Metadata *SessionMetadata `json:"metadata,omitempty"`
	Events   []StoredEvent    `json:"events,omitempty"`
}

func loadRecoveryFixture(t *testing.T, variant, session string) StoredSession {
	t.Helper()
	filename := filepath.Join("testdata", "recovery", variant, session+".jsonl")
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	var stored StoredSession
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for number := 1; scanner.Scan(); number++ {
		var line recoveryFixtureLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatalf("decoding %s line %d: %v", filename, number, err)
		}
		if line.Version != uint64(number) {
			t.Fatalf("%s line %d carries version %d", filename, number, line.Version)
		}
		if line.Metadata != nil {
			stored.Metadata = *line.Metadata
		}
		stored.Events = append(stored.Events, line.Events...)
		stored.Version = line.Version
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if stored.Metadata.ID == "" {
		t.Fatalf("%s carries no session metadata", filename)
	}
	return stored
}

// TestRecoveryProjectionClearsAppliedLegacyIntent is the primary red test. A
// real v1 intent whose recorded apply outcome is applied and whose git
// finalizer succeeded is a finished write: the store plainly records the
// desired state as reached. The projection must not report it as an actionable
// pending write, and must not demand a target binding for a write that no
// longer needs a target.
func TestRecoveryProjectionClearsAppliedLegacyIntent(t *testing.T) {
	stored := loadRecoveryFixture(t, "sessions", strandedFixtureSession)
	replay, err := replayRecovery(stored.Events, strandedFixtureMutationID)
	if err != nil {
		t.Fatal(err)
	}

	// Guard the fixture: this must be the real stranded shape, or the
	// assertions below prove nothing.
	if replay.prepared.Version != LegacyPreparedTransitionVersion {
		t.Fatalf("fixture prepared version = %d, want the legacy version %d", replay.prepared.Version, LegacyPreparedTransitionVersion)
	}
	if replay.apply.State != MutationApplied {
		t.Fatalf("fixture apply state = %q, want %q", replay.apply.State, MutationApplied)
	}
	if outcome, ok := replay.finalizers["git"]; !ok || !outcome.Succeeded {
		t.Fatalf("fixture git finalizer = %+v, present=%t, want a recorded success", outcome, ok)
	}
	if replay.terminal != nil || replay.bound != nil || replay.attempt != nil {
		t.Fatalf("fixture already carries recovery history: terminal=%+v bound=%+v attempt=%+v", replay.terminal, replay.bound, replay.attempt)
	}
	for _, change := range replay.prepared.Batch.Changes {
		if change.Document != nil {
			t.Fatalf("fixture change %q carries a structured Document; the real v1 writer wrote none", change.LogicalPath)
		}
	}

	item := recoveryItem(stored, replay)
	if item.Actionable {
		t.Errorf("recoveryItem(%s).Actionable = true, want false: the store records apply=%s with the git finalizer succeeded, so there is no pending write to recover",
			strandedFixtureMutationID, replay.apply.State)
	}
	if item.LegacyUnroutable {
		t.Errorf("recoveryItem(%s).LegacyUnroutable = true, want false: an already-applied write needs no target binding", strandedFixtureMutationID)
	}
	if item.State == RecoveryUnknown {
		t.Errorf("recoveryItem(%s).State = %q, want a state reflecting the recorded applied outcome, not unknown", strandedFixtureMutationID, item.State)
	}
}

// TestRecoveryProjectionKeepsAppliedLegacyIntentWithFailedFinalizerActionable
// pins the boundary the coming fix must not cross. The same real applied intent
// with a failed git finalizer is not a notice to clear: the finalizer is
// independently retryable, so the item stays actionable.
func TestRecoveryProjectionKeepsAppliedLegacyIntentWithFailedFinalizerActionable(t *testing.T) {
	stored := loadRecoveryFixture(t, "finalizer-failed", strandedFixtureSession)
	replay, err := replayRecovery(stored.Events, strandedFixtureMutationID)
	if err != nil {
		t.Fatal(err)
	}

	if replay.prepared.Version != LegacyPreparedTransitionVersion {
		t.Fatalf("fixture prepared version = %d, want the legacy version %d", replay.prepared.Version, LegacyPreparedTransitionVersion)
	}
	if replay.apply.State != MutationApplied {
		t.Fatalf("fixture apply state = %q, want %q", replay.apply.State, MutationApplied)
	}
	if outcome, ok := replay.finalizers["git"]; !ok || outcome.Succeeded {
		t.Fatalf("fixture git finalizer = %+v, present=%t, want a recorded failure", outcome, ok)
	}

	item := recoveryItem(stored, replay)
	if !item.Actionable {
		t.Errorf("recoveryItem(%s).Actionable = false, want true: the recorded git finalizer failed, so finalization is still owed",
			strandedFixtureMutationID)
	}
}
