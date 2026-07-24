package local

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	app "github.com/networkteam/sdd/application"
)

const (
	// SessionIdentityTransitionMarker is the bounded machine-global routing
	// hold for one identity-less-key to repo-ID-key transition.
	SessionIdentityTransitionMarker = ".identity-transition.json"

	SessionIdentityTransitionVersion = 1
)

type SessionIdentityTransitionState string

const (
	SessionIdentityTransitionPending   SessionIdentityTransitionState = "pending"
	SessionIdentityTransitionCutover   SessionIdentityTransitionState = "cutover"
	SessionIdentityTransitionCompleted SessionIdentityTransitionState = "completed"
)

// SessionIdentityTransition records one bounded identity change alongside the
// old global session store. Pending routes to Old*; cutover/completed route to
// Current* and never act as a general alias.
type SessionIdentityTransition struct {
	Version         int                            `json:"version"`
	State           SessionIdentityTransitionState `json:"state"`
	OldKey          string                         `json:"old_key"`
	NewKey          string                         `json:"new_key"`
	OldSessions     string                         `json:"old_sessions"`
	OldBlobs        string                         `json:"old_blobs"`
	CurrentSessions string                         `json:"current_sessions"`
	CurrentBlobs    string                         `json:"current_blobs"`
	TargetProject   app.ProjectID                  `json:"target_project"`
	UpdatedAt       time.Time                      `json:"updated_at"`
}

func SessionIdentityTransitionPath(oldSessions string) string {
	return filepath.Join(oldSessions, SessionIdentityTransitionMarker)
}

func ReadSessionIdentityTransition(oldSessions string) (*SessionIdentityTransition, error) {
	filename := SessionIdentityTransitionPath(oldSessions)
	encoded, present, err := readRegularControlFile(
		oldSessions, SessionIdentityTransitionMarker,
	)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, nil
	}
	return decodeSessionIdentityTransition(encoded, filename)
}

func readSessionIdentityTransitionRoot(
	root *os.Root,
	displayRoot string,
) (*SessionIdentityTransition, error) {
	filename := filepath.Join(displayRoot, SessionIdentityTransitionMarker)
	encoded, present, err := readRootedRegularControlFile(
		root, SessionIdentityTransitionMarker,
	)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, nil
	}
	return decodeSessionIdentityTransition(encoded, filename)
}

func readRegularControlFile(directory, name string) ([]byte, bool, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, false, err
	}
	chain, err := openTrustedAbsoluteDirectoryChain(absolute, false, true, 0)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = chain.close() }()
	root := chain.root()
	if root == nil {
		return nil, false, nil
	}
	encoded, present, err := readRootedRegularControlFile(root, name)
	if err != nil {
		return nil, false, err
	}
	if err := chain.revalidate(); err != nil {
		return nil, false, err
	}
	return encoded, present, nil
}

func readRootedRegularControlFile(root *os.Root, name string) ([]byte, bool, error) {
	file, err := openRootedRegular(root, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	opened, statErr := file.Stat()
	encoded, readErr := io.ReadAll(file)
	atName, nameErr := root.Lstat(name)
	closeErr := file.Close()
	if err := errors.Join(statErr, readErr, nameErr, closeErr); err != nil {
		return nil, false, err
	}
	if !opened.Mode().IsRegular() || !atName.Mode().IsRegular() ||
		!os.SameFile(opened, atName) {
		return nil, false, fmt.Errorf("rooted control %s changed while reading", name)
	}
	return encoded, true, nil
}

func decodeSessionIdentityTransition(encoded []byte, filename string) (*SessionIdentityTransition, error) {
	var transition SessionIdentityTransition
	if err := json.Unmarshal(encoded, &transition); err != nil {
		return nil, fmt.Errorf("decoding session identity transition marker %s: %w", filename, err)
	}
	if transition.Version != SessionIdentityTransitionVersion {
		return nil, fmt.Errorf("unsupported session identity transition marker version %d", transition.Version)
	}
	switch transition.State {
	case SessionIdentityTransitionPending, SessionIdentityTransitionCutover, SessionIdentityTransitionCompleted:
	default:
		return nil, fmt.Errorf("unsupported session identity transition state %q", transition.State)
	}
	if transition.OldKey == "" || transition.NewKey == "" ||
		transition.OldSessions == "" || transition.OldBlobs == "" ||
		transition.CurrentSessions == "" || transition.CurrentBlobs == "" ||
		transition.TargetProject == "" {
		return nil, fmt.Errorf("session identity transition marker %s is incomplete", filename)
	}
	return &transition, nil
}

func WriteSessionIdentityTransition(oldSessions string, transition SessionIdentityTransition) error {
	if transition.Version == 0 {
		transition.Version = SessionIdentityTransitionVersion
	}
	if transition.Version != SessionIdentityTransitionVersion {
		return fmt.Errorf("unsupported session identity transition marker version %d", transition.Version)
	}
	if err := os.MkdirAll(oldSessions, 0o755); err != nil {
		return err
	}
	transition.UpdatedAt = time.Now().UTC().Round(0)
	return writeJSONAtomic(SessionIdentityTransitionPath(oldSessions), transition)
}

func writeSessionIdentityTransitionRoot(root *os.Root, transition SessionIdentityTransition) error {
	if transition.Version == 0 {
		transition.Version = SessionIdentityTransitionVersion
	}
	if transition.Version != SessionIdentityTransitionVersion {
		return fmt.Errorf("unsupported session identity transition marker version %d", transition.Version)
	}
	transition.UpdatedAt = time.Now().UTC().Round(0)
	return writeJSONAtomicRoot(root, SessionIdentityTransitionMarker, transition)
}
