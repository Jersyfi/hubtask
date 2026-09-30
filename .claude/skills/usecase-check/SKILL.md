---
name: usecase-check
description: Check a branch, a pull request, a milestone or a single use case against Hubtask's use cases (docs/usecases) — every named check met or not with evidence, anything built beyond "Where it ends", principles broken on the way. Use before a pull request leaves draft, when the owner asks whether work meets its goal, and at the end of a milestone. Arguments - a PR number, a branch, a milestone id (SC), or a use case id (UC-ID-04).
---

# /usecase-check — does the work meet the goal it was cut for?

The use cases in `docs/usecases/` are the owner's statement of what must be true for a person. This
check reads the work **against them**, not against the task text or the session's own plan — that
is the whole point: a session that reinterprets a goal passes its own review and fails this one.

Read `docs/usecases/README.md` and `docs/vision/principles.md` first, completely.

## 1. Find what the work promised

| Argument | Where the promise is |
|---|---|
| a PR number | `gh pr view <n> --json body,headRefName,files` → the *Use cases* section; the task's `**Use cases:**` line in `docs/backlog/milestone-*.md` (the PR closes an issue whose title starts with the task id) |
| a branch | `git log main..<branch>` → the `Task:` trailers → the task's `**Use cases:**` line |
| a milestone id | `docs/backlog/milestone-<id>.md` → every task's `**Use cases:**` line |
| a use case id | the use case itself, against the code on `main` |

If the task names **no** use cases, say so first: that is itself the finding — the work was built
without a yardstick.

## 2. For every named check, decide met, not met, or not proven

Read each use case completely — *Goal*, *How to check*, *Where it ends*, *Today*. For each check the
work claims:

* **Find the evidence**: the test that asserts it (name the test function), the code path that makes
  it true (file:line), or a walk. Where a check describes a screen, run the app
  (`preview_start` with `webapp`, or the stub preview) and look as the persona in `actors:` would —
  at 375 px and by keyboard where the check says so.
* **Met** only with evidence. "The code looks like it would" is **not proven**, not met.
* **Not met**: say what the person would see instead.
* Check the *deployments:* too: a check that holds in `D6` and needs a platform concept in `D1`
  fails [P-10](../../../docs/vision/principles.md).

For a large scope, fan the use cases out to subagents (one per context folder) with this skill's
steps and the evidence rule; collect their tables.

## 3. Look for work beyond the goal

Compare the diff with every *Where it ends* section and with `docs/vision/non-goals.md`. Anything
built that a use case explicitly does not ask for — or that a non-goal forbids — is a finding, even
when it works. So is a check that was quietly rewritten: `git diff main -- docs/usecases` must not
touch *Goal*, *How to check* or *Where it ends* unless the pull request says the owner asked for it.

## 4. Look for principles broken on the way

For each principle the use cases serve, the "Broken when" line in `principles.md` is the test. The
ones sessions break most: a control offered that the server refuses (P-05), a second place for the
same rule (P-06), a raw key or identifier on a screen (P-12), a path that opens an account on less
than its own proof (P-02), a task only SQL can do (P-08, P-09).

## 5. Report

```markdown
## Use case check — <scope>, <date>

| Use case | Check | Verdict | Evidence / what the person sees instead |
|---|---|---|---|
| UC-ID-04 | 5 | not met | ResetView stays on the form after 202; the link is spent |

**Beyond the goal:** …
**Principles:** …
**States:** use cases whose `state:` / `checked_by:` / *Today* no longer match the verdicts above.
**Verdict:** ready / not ready — the one or two things that decide it.
```

Report in the language the owner used. Never edit a use case's *Goal*, *How to check* or *Where it
ends* to make a verdict come out right. When run unattended, findings become an issue (label
`task`, milestone of the work) and nothing is changed.
