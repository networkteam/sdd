// Command extract_fixtures trims real session logs from a live sdd session
// store into the small recovery-projection fixtures next to it.
//
// It lives under testdata (which the go tool ignores) because it is a
// regeneration utility, not part of the build. Run it explicitly:
//
//	go run ./application/testdata/recovery/extract_fixtures.go \
//	    -source "$HOME/.local/state/sdd/sessions/github.com/networkteam/sdd" \
//	    -out application/testdata/recovery \
//	    s_20260714-095955-885b3c45 s_20260715-165703-e25be2e2 s_20260716-121551-d740f1c6
//
// For each named session it keeps the session's create line (metadata only)
// plus the events of exactly ONE mutation — selected by the rule in select()
// below — and drops every other line, which in a real session is ~100+
// workflow_event entries plus the other mutations. Event bytes are copied
// verbatim: the output line is assembled as {"version":N,"events":[<raw>]}
// around the untouched source event, so payloads stay byte-identical to what
// the writer actually produced. Only the envelope "version" field is
// renumbered, because the store requires line N to carry version N.
//
// It additionally writes a finalizer-failed variant of the first session, which
// is the same fixture with the single byte-level substitution
// "Succeeded":true -> "Succeeded":false in the finalizer_outcome payload. No
// real session in the store records a failed git finalizer, so that one
// boundary case is derived rather than observed; everything else is observed.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	codeMutationIntent    = "mutation_intent"
	codeMutationOutcome   = "mutation_outcome"
	codeFinalizerOutcome  = "finalizer_outcome"
	codeRecoveryAttempt   = "recovery_attempt"
	codeRecoveryTerminal  = "recovery_terminal"
	codeLegacyTargetBound = "legacy_target_bound"
)

type envelope struct {
	Version  uint64            `json:"version"`
	Metadata json.RawMessage   `json:"metadata,omitempty"`
	Events   []json.RawMessage `json:"events,omitempty"`
}

type eventHead struct {
	Code    string `json:"Code"`
	Payload struct {
		MutationID string `json:"mutation_id"`
		Prepared   struct {
			Version uint32 `json:"Version"`
			Batch   struct {
				ID string `json:"ID"`
			} `json:"Batch"`
		} `json:"prepared"`
	} `json:"Payload"`
}

type event struct {
	raw  []byte
	head eventHead
}

func (e event) mutationID() string {
	if e.head.Payload.MutationID != "" {
		return e.head.Payload.MutationID
	}
	return e.head.Payload.Prepared.Batch.ID
}

func main() {
	source := flag.String("source", "", "directory holding the real session JSONL logs (required)")
	out := flag.String("out", ".", "fixture directory to write into")
	flag.Parse()
	if *source == "" || flag.NArg() == 0 {
		log.Fatal("usage: extract_fixtures -source <dir> [-out <dir>] <session-id>...")
	}
	for index, id := range flag.Args() {
		createLine, kept, mutationID, err := trim(filepath.Join(*source, id+".jsonl"))
		if err != nil {
			log.Fatalf("trimming %s: %v", id, err)
		}
		if err := write(filepath.Join(*out, "sessions", id+".jsonl"), createLine, kept); err != nil {
			log.Fatalf("writing %s: %v", id, err)
		}
		fmt.Printf("sessions/%s.jsonl: %s (%d events)\n", id, mutationID, len(kept))
		if index != 0 {
			continue
		}
		failed, err := failFinalizer(kept)
		if err != nil {
			log.Fatalf("deriving finalizer-failed variant of %s: %v", id, err)
		}
		if err := write(filepath.Join(*out, "finalizer-failed", id+".jsonl"), createLine, failed); err != nil {
			log.Fatalf("writing finalizer-failed %s: %v", id, err)
		}
		fmt.Printf("finalizer-failed/%s.jsonl: %s (%d events)\n", id, mutationID, len(failed))
	}
}

// trim reads one real session log and returns its verbatim create line, the
// verbatim events of the selected mutation, and that mutation's ID.
func trim(filename string) (createLine []byte, kept [][]byte, mutationID string, err error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, nil, "", err
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var events []event
	for line := 1; scanner.Scan(); line++ {
		raw := append([]byte(nil), scanner.Bytes()...)
		var decoded envelope
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, nil, "", fmt.Errorf("line %d: %w", line, err)
		}
		if line == 1 {
			if decoded.Version != 1 || decoded.Metadata == nil || len(decoded.Events) != 0 {
				return nil, nil, "", fmt.Errorf("line 1 is not a bare create line")
			}
			createLine = raw
		}
		for _, encoded := range decoded.Events {
			var head eventHead
			if err := json.Unmarshal(encoded, &head); err != nil {
				return nil, nil, "", fmt.Errorf("line %d event: %w", line, err)
			}
			events = append(events, event{raw: encoded, head: head})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, "", err
	}
	if createLine == nil {
		return nil, nil, "", fmt.Errorf("empty session log")
	}
	mutationID, err = selectMutation(events)
	if err != nil {
		return nil, nil, "", err
	}
	for _, candidate := range events {
		switch candidate.head.Code {
		case codeMutationIntent, codeMutationOutcome, codeFinalizerOutcome,
			codeRecoveryAttempt, codeRecoveryTerminal, codeLegacyTargetBound:
			if candidate.mutationID() == mutationID {
				kept = append(kept, candidate.raw)
			}
		}
	}
	return createLine, kept, mutationID, nil
}

// selectMutation is the fixture selection rule: the first mutation, in log
// order, whose intent records the legacy prepared version, whose apply outcome
// and at least one finalizer outcome were both recorded, and which no recovery
// event ever touched. That is precisely the stranded shape the live store is
// full of, and taking the first match keeps the choice mechanical.
func selectMutation(events []event) (string, error) {
	var order []string
	seen := map[string]bool{}
	legacy := map[string]bool{}
	outcome := map[string]bool{}
	finalizer := map[string]bool{}
	recovered := map[string]bool{}
	for _, candidate := range events {
		id := candidate.mutationID()
		if id == "" {
			continue
		}
		switch candidate.head.Code {
		case codeMutationIntent:
			if !seen[id] {
				seen[id] = true
				order = append(order, id)
			}
			legacy[id] = candidate.head.Payload.Prepared.Version == 1
		case codeMutationOutcome:
			outcome[id] = true
		case codeFinalizerOutcome:
			finalizer[id] = true
		case codeRecoveryAttempt, codeRecoveryTerminal, codeLegacyTargetBound:
			recovered[id] = true
		}
	}
	for _, id := range order {
		if legacy[id] && outcome[id] && finalizer[id] && !recovered[id] {
			return id, nil
		}
	}
	return "", fmt.Errorf("no stranded legacy mutation triple found")
}

// failFinalizer derives the boundary fixture by flipping the recorded git
// finalizer to failed. It is the only field this tool ever rewrites.
func failFinalizer(events [][]byte) ([][]byte, error) {
	derived := make([][]byte, 0, len(events))
	flipped := 0
	for _, raw := range events {
		var head eventHead
		if err := json.Unmarshal(raw, &head); err != nil {
			return nil, err
		}
		if head.Code != codeFinalizerOutcome {
			derived = append(derived, raw)
			continue
		}
		if !bytes.Contains(raw, []byte(`"Succeeded":true`)) {
			return nil, fmt.Errorf("finalizer outcome does not record success: %s", raw)
		}
		derived = append(derived, []byte(strings.Replace(string(raw), `"Succeeded":true`, `"Succeeded":false`, 1)))
		flipped++
	}
	if flipped != 1 {
		return nil, fmt.Errorf("expected exactly one finalizer outcome to flip, flipped %d", flipped)
	}
	return derived, nil
}

// write emits the trimmed log: the verbatim create line, then one line per kept
// event with a renumbered envelope version so line N carries version N.
func write(filename string, createLine []byte, events [][]byte) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	var buffer bytes.Buffer
	buffer.Write(createLine)
	buffer.WriteByte('\n')
	for index, raw := range events {
		fmt.Fprintf(&buffer, `{"version":%d,"events":[`, index+2)
		buffer.Write(raw)
		buffer.WriteString("]}\n")
	}
	return os.WriteFile(filename, buffer.Bytes(), 0o644)
}
