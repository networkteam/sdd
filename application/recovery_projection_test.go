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

// loadRecoveryFixture reads the stranded fixture session in the named variant
// directory; each variant holds the same session under a different outcome.
func loadRecoveryFixture(t *testing.T, variant string) StoredSession {
	t.Helper()
	filename := filepath.Join("testdata", "recovery", variant, strandedFixtureSession+".jsonl")
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

// TestRecoveryProjectionClearsAppliedLegacyIntent covers the stranded shape the
// live store holds. A real v1 intent whose recorded apply outcome is applied and
// whose git finalizer succeeded is a finished write: the store plainly records
// the desired state as reached. The projection must not report it as an
// actionable pending write, and must not demand a target binding for a write
// that no longer needs a target.
func TestRecoveryProjectionClearsAppliedLegacyIntent(t *testing.T) {
	stored := loadRecoveryFixture(t, "sessions")
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
	if item.Actionable() {
		t.Errorf("recoveryItem(%s).Actionable = true, want false: the store records apply=%s with the git finalizer succeeded, so there is no pending write to recover",
			strandedFixtureMutationID, replay.apply.State)
	}
	if item.LegacyUnroutable {
		t.Errorf("recoveryItem(%s).LegacyUnroutable = true, want false: an already-applied write needs no target binding", strandedFixtureMutationID)
	}
	if item.State != RecoveryDelivered {
		t.Errorf("recoveryItem(%s).State = %q, want %q: the recorded outcome proves the write reached its desired state", strandedFixtureMutationID, item.State, RecoveryDelivered)
	}
	if item.Reason != "" {
		t.Errorf("recoveryItem(%s).Reason = %q, want empty: a delivered write owes nothing", strandedFixtureMutationID, item.Reason)
	}
	if item.Recovered {
		t.Errorf("recoveryItem(%s).Recovered = true, want false: no recovery verb ever touched this mutation", strandedFixtureMutationID)
	}
}

// TestRecoveryProjectionKeepsAppliedLegacyIntentWithFailedFinalizerActionable
// pins one half of the delivery boundary. The same real applied intent with a
// failed git finalizer is not a notice to clear: the finalizer is independently
// retryable, so the item stays actionable.
func TestRecoveryProjectionKeepsAppliedLegacyIntentWithFailedFinalizerActionable(t *testing.T) {
	stored := loadRecoveryFixture(t, "finalizer-failed")
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
	if !item.Actionable() {
		t.Errorf("recoveryItem(%s).Actionable = false, want true: the recorded git finalizer failed, so finalization is still owed",
			strandedFixtureMutationID)
	}
}

// TestRecoveryProjectionKeepsAppliedIntentWithoutFinalizerRecordActionable pins
// the other half of the delivery boundary: silence is not proof. An applied
// mutation carrying no finalizer outcome at all landed in the graph with its
// commit still owed — the writer records each finalizer's outcome only after
// running it, so an absent record means none ran. Suppressing that notice would
// strand the write with no way to reach finalize-retry.
//
// The input is the real stranded triple with its finalizer outcome removed, so
// the shape stays derived from what the v1 writer actually produced rather than
// hand-assembled. The live store holds no such session today; this guards the
// case forward.
func TestRecoveryProjectionKeepsAppliedIntentWithoutFinalizerRecordActionable(t *testing.T) {
	stored := loadRecoveryFixture(t, "sessions")
	withoutFinalizer := make([]StoredEvent, 0, len(stored.Events))
	for _, event := range stored.Events {
		if event.Code == eventFinalizerOutcome {
			continue
		}
		withoutFinalizer = append(withoutFinalizer, event)
	}
	if len(withoutFinalizer) == len(stored.Events) {
		t.Fatalf("fixture %s carries no %s event to remove", strandedFixtureSession, eventFinalizerOutcome)
	}
	stored.Events = withoutFinalizer

	replay, err := replayRecovery(stored.Events, strandedFixtureMutationID)
	if err != nil {
		t.Fatal(err)
	}
	if replay.apply.State != MutationApplied {
		t.Fatalf("fixture apply state = %q, want %q", replay.apply.State, MutationApplied)
	}
	if len(replay.finalizers) != 0 {
		t.Fatalf("replay recorded %d finalizer outcomes, want none", len(replay.finalizers))
	}

	item := recoveryItem(stored, replay)
	if !item.Actionable() {
		t.Errorf("recoveryItem(%s).Actionable = false, want true: no finalizer outcome is recorded, so the commit is still owed",
			strandedFixtureMutationID)
	}
	if item.State != RecoveryPending {
		t.Errorf("recoveryItem(%s).State = %q, want %q", strandedFixtureMutationID, item.State, RecoveryPending)
	}
}

// TestRecoveryProjectionKeepsReconciledAppliedIntentActionable pins the
// reconciliation path. A mutation whose canonical outcome never became
// definitive, and which a recovery attempt later reconciled to applied, has by
// construction no finalizer outcome: the writer runs finalizers only from a
// definitive apply. It must stay actionable so finalize-retry remains reachable.
func TestRecoveryProjectionKeepsReconciledAppliedIntentActionable(t *testing.T) {
	stored := loadRecoveryFixture(t, "sessions")
	intentOnly := make([]StoredEvent, 0, len(stored.Events))
	for _, event := range stored.Events {
		if event.Code == eventMutationOutcome || event.Code == eventFinalizerOutcome {
			continue
		}
		intentOnly = append(intentOnly, event)
	}
	stored.Events = intentOnly

	attempt, err := storedEvent(eventRecoveryAttempt, recoveryAttemptEvent{
		MutationID: strandedFixtureMutationID,
		Reconciled: ApplyResult{State: MutationApplied, Revision: "sha256:reconciled"},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored.Events = append(stored.Events, attempt)

	replay, err := replayRecovery(stored.Events, strandedFixtureMutationID)
	if err != nil {
		t.Fatal(err)
	}
	if replay.apply.State != MutationUnknown {
		t.Fatalf("replay apply state = %q, want %q", replay.apply.State, MutationUnknown)
	}
	if replay.attempt == nil || replay.attempt.Reconciled.State != MutationApplied {
		t.Fatalf("replay attempt = %+v, want a reconciliation recording %q", replay.attempt, MutationApplied)
	}

	item := recoveryItem(stored, replay)
	if !item.Actionable() {
		t.Errorf("recoveryItem(%s).Actionable = false, want true: reconciliation proved the apply landed but no finalizer has run",
			strandedFixtureMutationID)
	}
	if item.State != RecoveryPending {
		t.Errorf("recoveryItem(%s).State = %q, want %q", strandedFixtureMutationID, item.State, RecoveryPending)
	}
	if !item.Recovered {
		t.Errorf("recoveryItem(%s).Recovered = false, want true: a reconciliation attempt is recorded", strandedFixtureMutationID)
	}
}

// TestRecoveryProjectionDoesNotCallOrdinaryWritesRecovered pins provenance to the
// only evidence that carries it. The ordinary write path closes an applied
// mutation with a terminal whose verb is `apply`, so terminal presence says
// nothing about whether recovery machinery ran — reading it as provenance labels
// every successful write recovered, which is what this guards.
func TestRecoveryProjectionDoesNotCallOrdinaryWritesRecovered(t *testing.T) {
	stored := loadRecoveryFixture(t, "sessions")
	terminal, err := storedEvent(eventRecoveryTerminal, recoveryTerminalEvent{
		MutationID: strandedFixtureMutationID,
		Verb:       RecoveryApply,
	})
	if err != nil {
		t.Fatal(err)
	}
	stored.Events = append(stored.Events, terminal)

	replay, err := replayRecovery(stored.Events, strandedFixtureMutationID)
	if err != nil {
		t.Fatal(err)
	}
	if replay.terminal == nil || replay.attempt != nil {
		t.Fatalf("replay terminal=%+v attempt=%+v, want a terminal and no attempt", replay.terminal, replay.attempt)
	}

	item := recoveryItem(stored, replay)
	if item.State != RecoveryDelivered {
		t.Errorf("recoveryItem(%s).State = %q, want %q", strandedFixtureMutationID, item.State, RecoveryDelivered)
	}
	if item.Recovered {
		t.Errorf("recoveryItem(%s).Recovered = true, want false: an ordinary write closes with an apply terminal and no recovery ever ran", strandedFixtureMutationID)
	}
	if item.Actionable() {
		t.Errorf("recoveryItem(%s).Actionable = true, want false", strandedFixtureMutationID)
	}
}
