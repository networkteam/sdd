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
	ErrorSessionOwnership       ErrorCode = "session_ownership_mismatch"
	ErrorSessionInUse           ErrorCode = "session_in_use"
	ErrorSessionConflict        ErrorCode = "session_conflict"
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
	Holder     *SessionHolder
	Cause      error
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
