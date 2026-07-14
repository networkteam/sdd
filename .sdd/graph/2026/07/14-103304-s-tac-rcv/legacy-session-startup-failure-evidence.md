# Legacy session startup failure — live evidence

Date: 2026-07-14
Participant: Christopher
Harness: Codex using the local sdd MCP server

## Reproduction

1. Invoked the engine and called `start_session`.
2. The server returned:
   `sdd: session s_20260703-094857-eee1a42c has non-sequential version 0 at line 1`
3. Called `list_sessions`; it returned the same error.
4. Called `info`; it still worked, confirming the MCP server itself was registered and reachable.

## On-disk finding

The project had 52 `.jsonl` files under `.sdd/sessions/`:

- 51 legacy engine event logs whose first records use the shape
  `{"v":1,"ts":"…","session":"…","seq":1,"event":"session_meta","data":{…}}`
- 1 current session-store record whose first record uses the shape
  `{"version":1,"metadata":{…}}`

The named legacy session was not malformed in its original format. Its five events were sequential and ended with an explicit `abandoned` event. The incompatibility arose because the current filesystem session store tried to decode the old event record as its new envelope type. The absent `version` field decoded to zero, then failed the invariant that line 1 must have version 1.

The current `FilesystemSessionStore.List` reads every `*.jsonl` record and returns on the first load error. Consequently, one legacy record blocks the complete listing. Session startup also traverses that listing, so the same record blocks opening a new dialogue even though an otherwise valid current-format session exists.

## Recovery performed

All 51 legacy JSONL logs were moved, without deletion, to `.sdd/sessions-legacy/`. The legacy lock file was preserved there as well. No graph data was changed. After the move, `start_session` opened a new engine dialogue successfully.

This workaround proves the outage is caused by unhandled format coexistence. It also demonstrates the friction: recovery currently requires source-level diagnosis and direct filesystem intervention before the engine can capture the engine failure itself.
