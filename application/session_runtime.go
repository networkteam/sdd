package application

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const SessionCodecVersion uint32 = 1

type ChooserKind string

const (
	ChooserGate  ChooserKind = "gate"
	ChooserAgent ChooserKind = "agent"
	ChooserUser  ChooserKind = "user"
)

var chooserHolderTTL = map[ChooserKind]time.Duration{
	ChooserGate:  2 * time.Minute,
	ChooserAgent: 5 * time.Minute,
	ChooserUser:  30 * time.Minute,
}

type BindSessionRequest struct {
	SessionID     SessionID
	MCPSessionID  string
	ClientName    string
	ClientVersion string
	Chooser       ChooserKind
	Takeover      bool
}

type SessionBinding struct {
	SessionID    SessionID
	Subject      string
	Project      ProjectID
	MCPSessionID string
	Generation   uint64
	Version      uint64
}

type BindSessionResult struct {
	Project ProjectRef
	Binding SessionBinding
	Created bool
}

type SessionSummary struct {
	ID          SessionID
	Label       string
	Participant string
	UpdatedAt   time.Time
	Holder      *SessionHolder
	HolderLive  bool
}

type ListSessionsResult struct {
	Project  ProjectRef
	Sessions []SessionSummary
}

func (a *Application) ListSessions(ctx context.Context, identity RequestIdentity, project ProjectID) (ListSessionsResult, error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return ListSessionsResult{}, err
	}
	stored, err := runtime.options.Sessions.List(ctx, SessionFilter{Subject: principal.Subject, Project: runtime.options.Project.ID})
	if err != nil {
		return ListSessionsResult{}, err
	}
	now := runtime.options.Now().UTC()
	result := ListSessionsResult{Project: runtime.options.Project, Sessions: make([]SessionSummary, 0, len(stored))}
	for _, session := range stored {
		if err := validateStoredSession(session); err != nil {
			return ListSessionsResult{}, err
		}
		summary := SessionSummary{ID: session.Metadata.ID, Label: session.Metadata.Label, Participant: session.Metadata.Participant, UpdatedAt: session.Metadata.UpdatedAt}
		if session.Metadata.Holder != nil {
			holder := *session.Metadata.Holder
			summary.Holder = &holder
			summary.HolderLive = holder.ExpiresAt.After(now)
		}
		result.Sessions = append(result.Sessions, summary)
	}
	return result, nil
}

func (a *Application) ReleaseSession(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding) error {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return err
	}
	stored, err := runtime.options.Sessions.Load(ctx, binding.SessionID)
	if err != nil {
		return err
	}
	if err := verifyBinding(stored, binding); err != nil {
		return err
	}
	now := runtime.options.Now().UTC().Round(0)
	metadata := stored.Metadata
	metadata.HolderHistory = append(metadata.HolderHistory, SessionHolderRecord{Holder: *metadata.Holder, EndedAt: now, Reason: "released"})
	metadata.Holder = nil
	metadata.UpdatedAt = now
	_, err = runtime.options.Sessions.Append(ctx, binding.SessionID, stored.Version, SessionAppend{Metadata: &metadata})
	return err
}

// BindSession establishes the user-level exclusive holder above SessionStore
// CAS. Expired holders are replaced and recorded; replacing a live holder
// requires an explicit takeover request.
func (a *Application) BindSession(ctx context.Context, identity RequestIdentity, project ProjectID, request BindSessionRequest) (BindSessionResult, error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return BindSessionResult{}, err
	}
	if request.SessionID == "" || request.MCPSessionID == "" {
		return BindSessionResult{}, fmt.Errorf("sdd: session ID and MCP session ID are required")
	}
	ttl, ok := chooserHolderTTL[request.Chooser]
	if !ok {
		return BindSessionResult{}, fmt.Errorf("sdd: unsupported chooser kind %q", request.Chooser)
	}
	now := runtime.options.Now().UTC().Round(0)
	stored, loadErr := runtime.options.Sessions.Load(ctx, request.SessionID)
	if loadErr != nil {
		if !errors.Is(loadErr, ErrSessionNotFound) {
			return BindSessionResult{}, loadErr
		}
		metadata := SessionMetadata{
			CodecVersion: SessionCodecVersion, ID: request.SessionID, Subject: principal.Subject,
			Project: runtime.options.Project.ID, Participant: principal.Participant, UpdatedAt: now,
			Holder: newSessionHolder(principal.Subject, request, 1, now, ttl),
		}
		created, err := runtime.options.Sessions.Create(ctx, metadata)
		if err != nil {
			return BindSessionResult{}, err
		}
		return BindSessionResult{Project: runtime.options.Project, Binding: bindingFrom(created), Created: true}, nil
	}
	if err := validateStoredSession(stored); err != nil {
		return BindSessionResult{}, err
	}
	if stored.Metadata.Subject != principal.Subject || stored.Metadata.Project != runtime.options.Project.ID {
		return BindSessionResult{}, &ApplicationError{Code: ErrorSessionOwnership, Message: "session identity and project are immutable"}
	}
	metadata := stored.Metadata
	current := metadata.Holder
	generation := uint64(1)
	if current != nil {
		generation = current.Generation
		same := current.Subject == principal.Subject && current.MCPSessionID == request.MCPSessionID
		live := current.ExpiresAt.After(now)
		if !same && live && !request.Takeover {
			holder := *current
			return BindSessionResult{}, &ApplicationError{Code: ErrorSessionInUse, Message: "session is live on another connection", Holder: &holder}
		}
		if !same {
			reason := "expired_takeover"
			if live {
				reason = "explicit_takeover"
			}
			metadata.HolderHistory = append(metadata.HolderHistory, SessionHolderRecord{Holder: *current, EndedAt: now, Reason: reason})
			generation++
		}
	}
	metadata.Holder = newSessionHolder(principal.Subject, request, generation, now, ttl)
	metadata.UpdatedAt = now
	next, err := runtime.options.Sessions.Append(ctx, request.SessionID, stored.Version, SessionAppend{Metadata: &metadata})
	if err != nil {
		return BindSessionResult{}, err
	}
	stored.Metadata = metadata
	stored.Version = next
	return BindSessionResult{Project: runtime.options.Project, Binding: bindingFrom(stored)}, nil
}

func newSessionHolder(subject string, request BindSessionRequest, generation uint64, now time.Time, ttl time.Duration) *SessionHolder {
	return &SessionHolder{
		Subject: subject, MCPSessionID: request.MCPSessionID, ClientName: request.ClientName,
		ClientVersion: request.ClientVersion, Generation: generation, LastActivity: now, ExpiresAt: now.Add(ttl),
	}
}

func bindingFrom(stored StoredSession) SessionBinding {
	holder := stored.Metadata.Holder
	if holder == nil {
		return SessionBinding{SessionID: stored.Metadata.ID, Subject: stored.Metadata.Subject, Project: stored.Metadata.Project, Version: stored.Version}
	}
	return SessionBinding{
		SessionID: stored.Metadata.ID, Subject: stored.Metadata.Subject, Project: stored.Metadata.Project,
		MCPSessionID: holder.MCPSessionID, Generation: holder.Generation, Version: stored.Version,
	}
}

func validateStoredSession(stored StoredSession) error {
	if stored.Metadata.CodecVersion != SessionCodecVersion {
		return &ApplicationError{Code: ErrorMigrationRequired, Message: "unsupported session codec version", Version: stored.Metadata.CodecVersion}
	}
	for _, event := range stored.Events {
		if event.CodecVersion != SessionCodecVersion {
			return &ApplicationError{Code: ErrorMigrationRequired, Message: "unsupported session event codec version", Version: event.CodecVersion}
		}
	}
	return nil
}

func verifyBinding(stored StoredSession, binding SessionBinding) error {
	if err := validateStoredSession(stored); err != nil {
		return err
	}
	holder := stored.Metadata.Holder
	if stored.Metadata.ID != binding.SessionID || stored.Metadata.Subject != binding.Subject || stored.Metadata.Project != binding.Project ||
		holder == nil || holder.Subject != binding.Subject || holder.MCPSessionID != binding.MCPSessionID || holder.Generation != binding.Generation {
		return &ApplicationError{Code: ErrorSessionOwnership, Message: "session holder was displaced"}
	}
	if stored.Version != binding.Version {
		return &ApplicationError{Code: ErrorSessionConflict, Message: "session binding is stale"}
	}
	return nil
}
