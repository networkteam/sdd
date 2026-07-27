# Recovery projection fixtures

Trimmed session logs taken from the **live** sdd session store on the machine
that produced them, used by `recovery_projection_test.go` (in-package
projection unit tests) and `recovery_projection_integration_test.go`
(store-level projection through `Application.ListRecoveries`).

These are real writer output, not hand-authored intents. That matters: the v1
writer recorded `"Document": null` on every change and never wrote a
`recovery_terminal`, and a hand-written v1 intent tends to get exactly that
combination wrong — an earlier round of this code shipped a recovery verb whose
proofs used constructed v1 intents *with* documents attached, so the verb was
unreachable on every real intent while its tests passed.

## Source

Store directory (macOS/XDG state root for project `github.com/networkteam/sdd`):

    $HOME/.local/state/sdd/sessions/github.com/networkteam/sdd

Source session logs:

| session | selected mutation | sha256 of the full source log |
| --- | --- | --- |
| `s_20260714-095955-885b3c45.jsonl` | `entry-20260714-103304-s-tac-rcv` | `502d14e7e5978c3082d66794569b76d08cf05aef602500d158dc12f6ffcadb83` |
| `s_20260715-165703-e25be2e2.jsonl` | `entry-20260715-172002-s-cpt-i5a` | `d2ce582655c37d6bcd0f2531662b93b92696d939871fb5e69fb956d974ef118f` |
| `s_20260716-121551-d740f1c6.jsonl` | `entry-20260716-122508-s-tac-xm3` | `dcd81cb9e2c6d06082f424b754382e2c2044ab7ab7d68ab6d11e9eec21e20832` |

Each source log carries 300+ lines, overwhelmingly `workflow_event`.

## Selection rule

Per session, the **first** mutation in log order whose

1. `mutation_intent` records `prepared.Version == 1` (legacy prepared version),
2. `mutation_outcome` was recorded,
3. at least one `finalizer_outcome` was recorded, and
4. no `recovery_attempt` / `recovery_terminal` / `legacy_target_bound` event
   ever touched it.

That is the stranded shape the live store is full of: at the time of extraction
all 34 non-terminal intents across the 47 session logs in that store matched it,
every one of them recording `apply.State == "applied"` with the `git` finalizer
succeeded, and none of them carrying a non-null `Document`.

## What the trimming does

Kept: the session's create line (metadata only) plus the events of the selected
mutation. Dropped: every other line.

Event bytes are **verbatim** — each output line is assembled as
`{"version":N,"events":[<raw source event>]}` around the untouched source event.
The only rewritten envelope field is `version`, renumbered so line N carries
version N, which `local.FilesystemSessionStore` requires.

`finalizer-failed/s_20260714-095955-885b3c45.jsonl` is the boundary fixture: the
same trimmed log with the single byte-level substitution
`"Succeeded":true` → `"Succeeded":false` in the `finalizer_outcome` payload. No
real session in the store records a failed git finalizer, so that one field is
derived; everything else in every fixture is observed.

## Regenerating

    go run ./application/testdata/recovery/extract_fixtures.go \
        -source "$HOME/.local/state/sdd/sessions/github.com/networkteam/sdd" \
        -out application/testdata/recovery \
        s_20260714-095955-885b3c45 s_20260715-165703-e25be2e2 s_20260716-121551-d740f1c6

The tool only reads the source directory. It lives under `testdata`, which the
go tool ignores, so it is not part of the build.
