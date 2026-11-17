package model

import "errors"

// ErrIncompleteWrite marks a failure after entry files were written: an entry
// left without its summary or title, or files left uncommitted. Its message
// names the command that completes the write; a cancellation must not hide
// it, since no later run completes it.
var ErrIncompleteWrite = errors.New("incomplete write")

// IncompleteWrite marks err as ErrIncompleteWrite, its message unchanged; nil
// stays nil.
func IncompleteWrite(err error) error {
	if err == nil {
		return nil
	}
	return incompleteWrite{err: err}
}

type incompleteWrite struct{ err error }

func (e incompleteWrite) Error() string      { return e.err.Error() }
func (e incompleteWrite) Unwrap() error      { return e.err }
func (incompleteWrite) Is(target error) bool { return target == ErrIncompleteWrite }
