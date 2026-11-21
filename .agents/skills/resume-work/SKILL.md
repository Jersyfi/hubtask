---
name: resume-work
description: Pick up a branch another worker left, or stop work so that another worker can pick it up. Use it at the start of a session on an existing branch, and before ending a session or handing over.
---

# Resume and stop work

Everything another worker needs is in the repository and on the draft pull request. A conversation
is not.

## Resuming

1. Read the draft pull request: its numbered steps and the last comment that names the next step.

   ```bash
   gh pr view --json title,body,comments
   ```

2. Compare the steps with what is committed:

   ```bash
   git log --oneline main..HEAD
   ```

3. If another session uses this checkout, work in a worktree of your own.
4. Run `make check`. A red result is the first thing to fix.
5. Continue at the first step that has no commit.

## While working

- Each gate runs on its own line. `make gate-docs | tail` hides a red gate from a following `&&`,
  and `git push | tail` hides a rejected push.
- A long run starts in the background, and its end arrives as a notification. A `sleep` or `until`
  loop pays for the whole context on every turn.
- `make gate-selftest` edits the tree. Nothing else runs beside it.

## Stopping

Stop before the context passes about 250,000 tokens, or when the session ends.

1. Commit what is finished, one concern per commit. Leave nothing uncommitted that the next worker
   needs.
2. Run `make check`.
3. Push, and read the output: a rejected push is not a push.
4. Name the next step on the draft pull request, in a comment or in its list of steps: what is
   done, what comes next, and anything that is known to be red or unverified.
