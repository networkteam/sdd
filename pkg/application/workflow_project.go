package application

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/networkteam/sdd/internal/engine"
)

// An instance targets a project: recorded when the start call names one,
// derived from the dispatching parent or the home project otherwise
// (d-cpt-yjc). The engine sees none of this — the fact lives in the session
// ledger as the instance-level counterpart of the branch binding.

type instanceProjectEvent struct {
	Instance string    `json:"instance"`
	Project  ProjectID `json:"project"`
}

func instanceProjectRecord(instance string, project ProjectID) (StoredEvent, error) {
	payload, err := json.Marshal(instanceProjectEvent{Instance: instance, Project: project})
	if err != nil {
		return StoredEvent{}, err
	}
	return StoredEvent{CodecVersion: SessionCodecVersion, Code: workflowInstanceProjectCode, Payload: payload}, nil
}

func (w *WorkflowSession) restoreInstanceProjects(events []StoredEvent) error {
	for _, stored := range events {
		if stored.Code != workflowInstanceProjectCode {
			continue
		}
		if !SupportedSessionCodecVersion(stored.CodecVersion) {
			return &ApplicationError{Code: ErrorMigrationRequired, Message: "unsupported instance project event codec", Version: stored.CodecVersion}
		}
		var event instanceProjectEvent
		if err := json.Unmarshal(stored.Payload, &event); err != nil {
			return fmt.Errorf("decoding instance project event: %w", err)
		}
		if event.Instance == "" || event.Project == "" {
			return fmt.Errorf("decoding instance project event: instance and project are required")
		}
		w.instanceProjects[event.Instance] = event.Project
	}
	return nil
}

// instanceProject resolves the project an instance targets: its own record,
// else the nearest recorded ancestor's, else the home project.
func (w *WorkflowSession) instanceProject(instance string) ProjectID {
	if w.session == nil {
		return w.project
	}
	for id := instance; id != ""; {
		if project, ok := w.instanceProjects[id]; ok {
			return project
		}
		inst, ok := w.session.Instance(id)
		if !ok {
			break
		}
		id = inst.Parent
	}
	return w.project
}

// projectFor resolves the target project of the instance whose store a
// registry function runs over — the engine hands functions the store, not
// the instance.
func (w *WorkflowSession) projectFor(store *engine.Store) ProjectID {
	if w.session == nil {
		return w.project
	}
	for _, inst := range w.session.Instances() {
		if inst.Store == store {
			return w.instanceProject(inst.ID)
		}
	}
	return w.project
}

// startProjectFor settles the project a start call pins, validated before the
// instance exists; empty leaves the instance to derive its project.
func (w *WorkflowSession) startProjectFor(explicit ProjectID) (ProjectID, error) {
	if explicit == "" {
		return "", nil
	}
	if err := w.authorizeTarget(explicit, AccessRead); err != nil {
		return "", err
	}
	return explicit, nil
}

// targetRuntime resolves the runtime of a project an instance targets, with
// the access the operation needs. The home project resolves directly; another
// project passes the two grants of d-cpt-yjc.
func (w *WorkflowSession) targetRuntime(project ProjectID, required Access) (*ProjectRuntime, error) {
	_, home, err := w.app.resolve(w.ctx, w.identity, w.project, AccessRead)
	if err != nil {
		return nil, err
	}
	if project == "" || project == w.project {
		if required == AccessRead {
			return home, nil
		}
		_, runtime, err := w.app.resolve(w.ctx, w.identity, w.project, required)
		return runtime, err
	}
	_, runtime, err := w.app.resolveTargetProject(w.ctx, w.identity, home, project, required)
	return runtime, err
}

// authorizeTarget is targetRuntime without the runtime — the gate a write in
// another project passes before the application resolves it again for the
// write itself. At home it costs nothing.
func (w *WorkflowSession) authorizeTarget(project ProjectID, required Access) error {
	if project == "" || project == w.project {
		return nil
	}
	_, err := w.targetRuntime(project, required)
	return err
}

// ReadScope resolves where a free read runs: the home project on the session's
// branch binding by default, or another project the session may work in — on
// that project's configured default, since the binding is a fact about the
// home checkout alone.
func (w *WorkflowSession) ReadScope(ctx context.Context, identity RequestIdentity, project ProjectID) (ProjectID, string, bool, error) {
	w.setOperation(ctx, identity)
	if project == "" || project == w.project {
		return w.project, w.branch, w.branch != "", nil
	}
	if err := w.authorizeTarget(project, AccessRead); err != nil {
		return "", "", false, err
	}
	return project, "", false, nil
}

// resolveTargetProject resolves a project an instance of a session homed in
// home may work in. Two grants, kept distinct: the target lies in the home
// project's declared dependency closure — a graph fact, true for every
// principal — and the principal is a member of the target, asked with the
// access the operation needs. Reading a dependency inside the home view and
// being in it are different questions; this is the second.
func (a *Application) resolveTargetProject(ctx context.Context, identity RequestIdentity, home *ProjectRuntime, target ProjectID, required Access) (Principal, *ProjectRuntime, error) {
	principal, err := a.resolvePrincipal(ctx, identity)
	if err != nil {
		return Principal{}, nil, err
	}
	if target == "" || target == home.options.Project.ID {
		runtime, err := a.resolveProject(ctx, principal, home.options.Project.ID, required)
		return principal, runtime, err
	}
	if err := a.inDependencyClosure(ctx, principal, home, target); err != nil {
		return Principal{}, nil, err
	}
	runtime, err := a.resolveProject(ctx, principal, target, required)
	if err != nil {
		return Principal{}, nil, err
	}
	return principal, runtime, nil
}

// inDependencyClosure walks the declared dependencies transitively from home
// through the configurations the composition can reach. A dependency the
// composition cannot resolve is not part of the reachable closure, so nothing
// behind it is a valid target. Membership is a property of the resolved
// project, never of the declared string: a declaration names a repo ID, and
// only the composition knows which project carries it.
func (a *Application) inDependencyClosure(ctx context.Context, principal Principal, home *ProjectRuntime, target ProjectID) error {
	seen := map[ProjectID]bool{home.options.Project.ID: true}
	queue := []*ProjectRuntime{home}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, dependency := range current.options.Dependencies {
			runtime, err := a.access.ResolveDependency(ctx, principal, current.options.Project.ID, dependency)
			if err != nil || runtime == nil {
				continue
			}
			id := runtime.options.Project.ID
			if id == target {
				return nil
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			queue = append(queue, runtime)
		}
	}
	return &ApplicationError{Code: ErrorProjectUnavailable, Message: fmt.Sprintf("project %s is not in the declared dependency closure of %s", target, home.options.Project.ID)}
}
