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
section number — search the documents and the comments:

```bash
git grep -n "<name>" -- docs '*AGENTS.md' '*.go' '*.ts' '*.svelte' '*.mjs' '*.sql'
```

Rewrite or delete each sentence that is false from this change on. Do not append a correcting
paragraph or an "update" note. A sentence that is true and only looks unnecessary stays
(`docs/README.md`, "How to write").

## 3. What the change left behind

- Remove what the change made unused — a function, a flag, a translation key, a stub answer — in
  a commit of its own.
- A test is deleted or weakened only when the behaviour it asserts was removed or changed on
  purpose, or another test asserts the same thing. Name each such test in the description, with
  which of the two reasons applies.

## 4. The local check

```bash
make check
```

On its own line, green. Then push, and read the output of the push.

## 5. The independent review

Start a fresh agent that has not seen this conversation and give it
`.agents/skills/review-change/SKILL.md`. With Claude Code that is the subagent `reviewer`; with
another tool it is a new session. Fix each finding, or answer it with a reason. The reviewer reads
the answers once.

## 6. The outside reviewer

Where the outside reviewer's app is installed on the repository, ask it with the pull request
comment `@coderabbitai review`. If it does not review a draft, the request goes out together with
step 8.

- Read its comments. Fix what is right, in commits of their own. Answer the rest in one line.
- Do not teach it a rule by replying. A rule goes into the repository, where every worker and the
  reviewer read it.
- It reviews a limited number of files. For a diff above that limit the independent review stands
  alone, and the description says so.
- It comments and decides nothing. Its check is not required (`ci-cd.md` §5).

## 7. The description

A copy of `.github/PULL_REQUEST_TEMPLATE.md` with every section. A section with nothing to say
reads "None".

- *Premises*: each false premise corrected, each finding fixed on the way, each choice taken
  without asking.
- *Review*: who reviewed (the model and the date); findings found, fixed and left open, each open
  one with its reason; one row for the outside reviewer: comments, fixed, declined.
- It closes its issue (`Closes #n`), or says `No issue:` and why.

```bash
make gate-pr BODY=<file> TITLE="<title>"
```

## 8. Ready

```bash
gh pr ready
```

Rework after this step returns the pull request to draft first: `gh pr ready --undo`.
