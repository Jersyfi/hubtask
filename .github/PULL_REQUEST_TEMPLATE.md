<!-- The job "Pull request description" reads this (tools/checkpr, `make gate-pr BODY=<file>`):
     every section stays, in this order, and says n/a where it does not apply. Writing the
     description with `gh pr create --body` skips this template - start from a copy of it. -->

## What and why

<!-- Keep it short. The why matters more than the what — the what is in the diff. -->

Closes #

<!-- Or, when there is no issue: `No issue: <why>` on a line of its own. -->

## Use cases

<!-- The checks this pull request makes true — the ones the task carries in its `**Use cases:**`
     line, or the concept's in concept mode. One line per check: met or not, and the test that
     proves it. A check that
     cannot be met is reported here and in a `decision` or `finding` issue; it is never rewritten to
     match the code. If this pull request changes a use case's Goal, How to check or Where it ends,
     say per change: `correction` (a wrong reference or number) or `decision #<issue>`. Move
     `state:` and `checked_by:` in this same pull request. "n/a" only for a change no use case
     describes (tooling, CI, documents). -->

- UC-…: check n — met / not met — confirmed by …

## Premises

<!-- What the task, its use cases' *Today* and the documents said about today's code that turned
     out false when you checked it against the code, each with where you found it and what you
     corrected in this pull request (AGENTS.md, "The loop for every task"). "none false — checked:
     <what>" when everything held. -->

- none false — checked: …

## Does this need an ADR?

<!-- The rule lives in a subject document; an ADR records why. A new decision brings its ADR and
     its subject-document change in this same pull request. -->

- [ ] No — this implements a rule already decided. Where it lives: `<file>.md §n`, or ADR-….
- [ ] Yes, and it is in this pull request (with the subject-document change) or already merged: ADR-….
- [ ] It touches an item of AGENTS.md, "What you do not decide yourself" — **not decided in a
      pull request**: a `decision` issue first.

## Impact

- **Breaking change:** yes / no — if yes: the migration path
- **Security:** the threats touched (T-xx), or "none"
- **Data protection:** new processing of personal data? Purpose and retention
- **Operations:** new configuration, new dependency, new alerts?
