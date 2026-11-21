---
name: finish-pull-request
description: Take a draft pull request to Ready — proof, the documents the change makes false, cleanup, the independent review, the outside reviewer, the description. Use it when the build is done and before running gh pr ready.
---

# Finish a pull request

A pull request leaves draft once every step here is done. CI runs after that, and the merge waits
for the owner's word.

## 1. Proof

- Every use case check the work carries is proven by a test that asserts it
  (`docs/usecases/README.md`).
- Each such use case has its `state:`, its `checked_by:` and its *Today* moved.

## 2. The sentences the change made false

For each name the change renames or removes — an operation, a setting, a table, a message code, a
section number — search every tracked file:

```bash
git grep -n "<name>"
```

That reaches each place a sentence can sit: `docs/`, every `AGENTS.md`, the skills under
`.agents/skills/`, the Markdown files at the root (`README.md`, `CONTRIBUTING.md`, `SECURITY.md`),
the help comments of the `Makefile`, the workflows under `.github/`, `scripts/`, and the comments
in the code.

Rewrite or delete each sentence that is false from this change on. Do not append a correcting
paragraph or an "update" note. A sentence that is true and only looks unnecessary stays
(`docs/README.md`, "How to write").

## 3. What the change left behind

- Remove what the change made unused — a function, a flag, a translation key, a stub answer — in
  a commit of its own.
- A test is deleted or weakened only when the behaviour it asserts was removed or changed on
  purpose, or another test asserts the same thing. Name each such test in the description, with
  which of the two reasons applies.

## 4. The author's own check

Read the change against every rule of `AGENTS.md`, and of the touched directories' `AGENTS.md`,
that is tagged `[partial]`, `[unchecked]` or `[owner]`: a gate holds these in part or not at all.
Fix what that finds.
The independent review of step 5 is added to this check and does not replace it
(`engineering-guidelines.md` §3.19).

Then run the local check:

```bash
make check
```

Run it on a line of its own, without a pipe, and go on only when it is green. Then push, and read
the output of the push: a rejected push is not a push.

## 5. The independent review

Start a fresh agent that has not seen this conversation and give it
`.agents/skills/review-change/SKILL.md`. With a tool that reads `.claude/agents/`, that is the
subagent `reviewer`; with another tool it is a new session. Fix each finding in a commit of its own, or answer it with a
reason. The reviewer reads the answers once.

## 6. The outside reviewer

Where the outside reviewer's app is installed on the repository, ask it with the pull request
comment `@coderabbitai review`. If it does not review a draft, the request goes out together with
step 9; a fix for one of its comments then returns the pull request to draft first
(`gh pr ready --undo`) and goes through step 7 again.

- Read its comments. Fix what is right, in commits of their own. Answer the rest in one line.
- Do not teach it a rule by replying. A rule goes into the repository, where every worker and the
  reviewer read it.
- It reviews a limited number of files. For a diff above that limit the independent review stands
  alone, and the description says so.
- It comments and decides nothing. Its check is not required (`ci-cd.md` §5).

## 7. After the last fix

A commit made after step 4 — a fix for the independent review, a fix for the outside reviewer —
has not been checked and is not pushed. When the last fix is committed, run both again, each on a
line of its own:

```bash
make check
git push
```

Read the output of the push, then compare the local commit with the pushed one:

```bash
git rev-parse HEAD @{upstream}
```

The two lines show the same commit when everything is pushed. This step holds with every tool:
the hook that refuses `gh pr ready` before `HEAD` is pushed exists for one tool, and it does not
run `make check`.

## 8. The description

A copy of `.github/PULL_REQUEST_TEMPLATE.md` with every section. A section with nothing to say
reads "n/a".

- *Premises*: each false premise corrected, each finding fixed on the way, each choice taken
  without asking.
- *Review*: who reviewed (the model and the date); findings found, fixed and left open as numbers,
  each open one with its reason; one row for the outside reviewer: comments, fixed, declined, or
  `not run:` and why.
- Each test the change removed or skipped, by its name, with which of the two reasons applies
  (step 3); a deleted test file by the file's name.
- It closes its issue (`Closes #n`), or says `No issue:` and why.

```bash
make gate-pr BODY=<file> TITLE="<title>"
```

## 9. Ready

Only after step 7: `make check` is green on `HEAD`, and `HEAD` is pushed.

```bash
gh pr ready
```

Rework after this step returns the pull request to draft first: `gh pr ready --undo`.
