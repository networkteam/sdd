package application

import (
	"errors"
	"fmt"
)

var ErrSessionNotFound = errors.New("sdd: session not found")

type ErrorCode string

const (
	ErrorAuthenticationRequired ErrorCode = "authentication_required"
	ErrorInvalidArgument        ErrorCode = "invalid_argument"
	ErrorProjectRequired        ErrorCode = "project_required"
	ErrorProjectUnavailable     ErrorCode = "project_unavailable"
	ErrorActionRequired         ErrorCode = "action_required"
	ErrorReadDenied             ErrorCode = "read_denied"
	ErrorWriteDenied            ErrorCode = "write_denied"
	ErrorBranchUnavailable      ErrorCode = "branch_unavailable"
	ErrorSessionOwnership       ErrorCode = "session_ownership_mismatch"
	ErrorSessionConflict        ErrorCode = "session_conflict"
	ErrorSessionDisplaced       ErrorCode = "session_displaced"
	ErrorConsentRequired        ErrorCode = "consent_required"
	ErrorGraphConflict          ErrorCode = "graph_conflict"
	ErrorMigrationRequired      ErrorCode = "migration_required"
	ErrorRecoveryRequired       ErrorCode = "recovery_required"
)

type ApplicationError struct {
	Code       ErrorCode
	Message    string
	Project    ProjectRef
	Action     *ProjectAction
	ApplyState ApplyState
	Revision   string
	Version    uint32
	Cause      error
	// Attachment and AttachmentCause carry the interpreted conflict on an
	// ErrorSessionDisplaced: who now holds the session (or the record that ended
	// it) and how it ended, so the displaced writer can be told who/when/why.
	Attachment      *Attachment
	AttachmentCause AttachmentCause
}

func (e *ApplicationError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Cause)
	}
	return string(e.Code)
}

func (e *ApplicationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
