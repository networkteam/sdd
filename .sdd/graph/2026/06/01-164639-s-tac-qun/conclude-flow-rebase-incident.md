# Incident: rebase-sync orphaned a merged branch during conclude

## What happened
An agent concluding a worktree-based plan did the right sequence: synced base
into the branch, verified build/test/lint, merged the branch into main
(fast-forward), captured the done signal, removed the worktree, then ran
`sdd wip done`. `wip done` removed the marker but **refused to delete the
branch**, reporting "unmerged changes" — even though it had just been
fast-forwarded into main.

## Root cause
Between the merge and `wip done`, SDD's background sync ran `git pull --rebase`.
The rebase replayed main's local commits onto origin's advanced tip, giving
them **new hashes** and orphaning the old branch tip.
`git merge-base --is-ancestor <branch> main` then returned false — a **false
negative** — because the check tests literal commit ancestry, not content. The
branch's content was verifiably on main (the new files present, tests green),
but the commit object was no longer an ancestor.

## Recovery
The agent did manual forensics — `git cat-file -e`, ancestry checks,
`git show-ref` — confirmed the content was on main, and force-deleted the
stale branch. Nothing was lost, but recovery depended on a careful agent.

## Lessons
1. `wip done`'s merged check must be **content-based** (patch-equivalence),
   not ancestry-based.
2. The multi-step conclude flow is **not atomic** against the background sync,
   which can rewrite history on its cooldown between steps.
3. Switching sync to **merge-pull** removes the self-inflicted local case
   (merge never rewrites); the content-based check covers manual/remote
   rebases.
4. Strong evidence that **tool-level guards beat agent-followed text
   guidance** — recovery hinged on meticulousness a guard would make
   unnecessary.
