# Auto-commit stall under `sdd serve` (Codex) — diagnosis

## Symptom

During a Codex CLI session running the workflow engine, confirming a capture playback triggered the auto-commit, which hung. The entry file and its attachment were written to disk but never committed. The `next` MCP call returned only after 300s with a generic timeout; the underlying commit subprocess stayed stuck and had to be killed by hand.

## Root cause

The auto-commit's `git commit` invoked SSH commit signing, whose signer blocked on an interactive passphrase prompt it could not satisfy, and — being a backgrounded child of a server that held a controlling terminal — was suspended by SIGTTIN instead of failing.

Process tree at inspection (all three stopped, STAT `T`):

    48981 sdd serve            (holds /dev/ttys000)                        STAT T
     +- 55043 git commit -m "sdd: decision tactical ..."                   STAT T
         +- 55044 ssh-keygen -Y sign -n git -f ~/.ssh/id_ed25519.pub ...   STAT T

Host git signing config (global ~/.gitconfig):

- commit.gpgsign = true
- gpg.format = ssh
- user.signingkey = ~/.ssh/id_ed25519.pub

Key facts:

- id_ed25519 is passphrase-protected.
- ~/.ssh/config: UseKeychain yes + AddKeysToAgent yes -> macOS keychain serves the passphrase via the launchd ssh-agent, so signing is normally silent.
- The agent holds exactly the signing key (fingerprint match: SHA256:/44uEPuWmVncDAEDqCxowxRmQg63HVIE9Y1U+mj+Slo).

## Why it hangs here but not under Claude Code

Signing needs the private key. Normally `ssh-keygen -Y sign` reaches the ssh-agent via SSH_AUTH_SOCK and uses the already-unlocked key — no prompt. Claude Code forwards SSH_AUTH_SOCK to the MCP server it launches, so signing succeeds.

Codex launches the MCP server (`.codex/config.toml` -> `[mcp_servers.sdd]`) with a filtered environment (`codex mcp get sdd` shows `env: -`; the default env policy passes only a "core" set, which excludes SSH_AUTH_SOCK). With no agent reachable, ssh-keygen falls back to the encrypted key file, needs the passphrase, and tries the controlling terminal the server inherited (/dev/ttys000). As a background process, that read raises SIGTTIN -> suspended -> the commit never returns.

(The MCP server runs as a plain child of codex, outside the per-command seatbelt sandbox that wraps shell tool calls — so this is a missing-env issue, not the sandbox blocking the agent socket. Not fully confirmed from the log; `ps eww -p $(pgrep -f 'sdd serve')` in a Codex session would settle it.)

## The failure is a hang, not an error

`gitCommit` (cmd/sdd/main.go) uses plain `exec.Command` — no timeout — and inherits the controlling terminal. The existing design (s-prc-1yb) degrades gracefully on commit *failure* (warn, keep the entry). A hang is not a failure, so that safety net never fires; the tool call blocks to the client's 300s timeout and the stuck subprocess is not reaped.

## Fail-fast lever (proven)

Signing behavior of `ssh-keygen -Y sign` on this machine:

| Condition | Result |
|---|---|
| agent reachable (normal SSH_AUTH_SOCK) | signs instantly |
| agent unreachable (SSH_AUTH_SOCK unset), no controlling TTY | fails in ~0s ("incorrect passphrase") |
| agent unreachable, controlling TTY, backgrounded | SIGTTIN -> suspended (the hang) |

So removing the controlling terminal turns the hang into an immediate failure.

## Remedies

- **Code-side (durable):** run the commit subprocess detached from any controlling terminal, under a context timeout, and propagate the error instead of swallowing it. Captured as a directive (fail fast + forward, all auto-commit paths).
- **Environment-side (Codex):** forward the agent socket to the server via `env_vars = ["SSH_AUTH_SOCK"]` in the generated `.codex/config.toml`. Belongs in the pending init Codex-registration work (d-tac-wfl). Stopgap only — verify `env_vars` is recognized by the installed Codex build.
