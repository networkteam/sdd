# Cross-repo reference architecture

## Commitments

- **Repo identifier:** URL-shaped (FQDN-style), declared per-repo in `.sdd/config.yaml` as a `repo_id:` field. Examples: `github.com/networkteam/sdd`, `gitlab.example.com/team/marketing`. Not derived from `git remote`; the declared value is the canonical identity, which survives hosting moves and works for local-only repos.
- **Mapping file:** per-user home-dir file (XDG-style, e.g. `~/.config/sdd/repos.yaml`) mapping repo ID → local checkout path. Maintained by the user.
- **Ref syntax:** standard `refs:` list entries with `<repo-id>:<entry-id>` inside the JSON `id`. Example: `--refs '{"id":"github.com/networkteam/sdd:20260517-170936-s-tac-7uy","kind":"grounds"}'`.
- **Ref kinds:** all eight kinds (`grounds`, `builds-on`, `refines`, `addresses`, `surfaces`, `evidence`, `depends-on`, `related`) are allowed across the boundary. The kind label carries the relationship's meaning for readers and the agent.
- **Lifecycle scope:** lifecycle derivation never crosses the boundary. The only ref kind with derivation effect today is `refines` (co-closure with the target); cross-repo `refines` keeps its label as documentation but local status is always computed from local signals.
- **Direction:** one-way forward only. The local entry's frontmatter names the remote ID; the remote graph is never written to.
- **Identity:** participant canonicals are strictly per-graph for v1. The same person appearing in multiple repos relies on discipline (matching canonicals or aliases in each repo's actor signals); the system does not unify across boundaries.

## Alternatives considered

### Identifier format

- **URL-shaped declared:** *chosen.* Readable, conventional (matches Git remote URLs and Go module paths), decoupled from actual git remote so it survives hosting changes. Works for repos without a remote.
- **Opaque UUID:** rejected. Collision-proof but ugly in refs and dialogue; loses the at-a-glance "which repo is this" signal.
- **Free-form string:** rejected. Maximum flexibility but global uniqueness depends entirely on user discipline, with no convention to anchor on.

### Write direction

- **One-way forward:** *chosen.* Each graph stays independent, no shared write coordination, immutability preserved cleanly across the boundary.
- **Bidirectional with back-references:** rejected for v1. Impractical to record on both sides without coupling write ordering. The "who cited me" view is recoverable at a higher layer (a meta-project that scans a configured set of repos to compute backlinks on demand), which keeps each per-repo graph simple.

### Lifecycle effects of `refines`

- **Forbid `refines` cross-repo:** initial instinct, rejected.
- **Scope the effect, not the kind:** *chosen.* All eight kinds work cross-repo. The only kind with a derivation effect (`refines` co-closure) has that effect declared to stay within-repo only. Cross-repo `refines` keeps its label as semantic documentation but never derives local status from remote state. This dissolves the missing-checkout worry — no cross-repo ref ever touches local status, so a missing remote checkout only means the agent can't read the cited content right now, not that local status is unknown.

### Identity unification

- **Per-graph for v1:** *chosen.* Participant canonicals are scoped to their declaring graph. The same person across repos relies on discipline (matching canonicals or aliases in each repo's actor signals) to stay recognizable.
- **System-side unification:** rejected for v1. Would require a global identity layer (e.g. a cross-repo actor registry) that adds substantial machinery for a problem the discipline solves adequately.

## Open questions (for the plan stage)

- **Mapping file format:** YAML structure, validation, what `sdd init` writes vs. what the user maintains.
- **CLI surface:** does `sdd show <repo-id>:<entry-id>` resolve via the mapping automatically? Does `sdd search` get a `--repo` flag for cross-repo querying? Other commands?
- **Pre-flight on cross-repo refs:** with the mapping + checkout available, the LLM advisory checks could read the remote entry. Without, syntactic check only. Where's the fallback line?
- **Render in catch-up and `sdd view`:** how do cross-repo refs render so they're visually distinct from local refs? Repo prefix shown, hidden, abbreviated?
- **Skill awareness:** which playbooks need updates (engage to follow refs across; capture to suggest cross-repo grounding; catch-up to render; etc.)?
- **Annotations across repos:** can a local annotation point at a remote entry? Lifecycle-scope rule says yes-with-no-derivation-effect, but the topic-membership computation may need explicit treatment.
- **Sync expectations:** does the agent ever auto-pull a referenced repo, or always best-effort against whatever's checked out?
