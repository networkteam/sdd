package model

import "errors"

// ErrUncommittedWrite marks a failure that left written entry files
// uncommitted. A cancellation must not hide it: no later run commits them.
var ErrUncommittedWrite = errors.New("the written entry files stay uncommitted")
