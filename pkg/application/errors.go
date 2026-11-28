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
	ErrorSessionEnded           ErrorCode = "session_ended"
	ErrorGraphConflict          ErrorCode = "graph_conflict"
	ErrorMigrationRequired      ErrorCode = "migration_required"
	ErrorRecoveryRequired       ErrorCode = "recovery_required"
	// ErrorOperationPending refuses a transition while a recorded operation is
	// unfinished; only its retry or cancellation is accepted (d-tac-t6u).
	ErrorOperationPending ErrorCode = "operation_pending"
	// ErrorRevisionUnavailable refuses a read whose requested revision cannot
	// be acquired; ApplicationError.Revision names it. Cancellation and
	// failures with their own code keep them.
	ErrorRevisionUnavailable ErrorCode = "revision_unavailable"
	// ErrorEntryNotFound refuses a read naming an ID that matches no entry;
	// ApplicationError.EntryID names it.
	ErrorEntryNotFound ErrorCode = "entry_not_found"
)

type ApplicationError struct {
	Code     ErrorCode
	Message  string
	Project  ProjectRef
	Action   *ProjectAction
	Revision string
	EntryID  string
	Version  uint32
	Cause    error
	// Ended carries the act that ended the session on an ErrorSessionEnded, so
	// the caller can be told who/when/why.
	Ended *SessionEnd
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
