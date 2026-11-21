---
name: review-change
description: Review a change as a fresh agent that did not build it — against its use cases, the rules no gate checks, the test rules, its design brief, its mockups, the documents it makes false and what it left behind — and report a table of findings. Use it before a pull request leaves draft, at each checkpoint of a concept, and for a pull request from a fork.
---

# Review a change

You review a change you did not build. You start with a fresh context and never see the builder's
conversation. You change no file, commit nothing and push nothing; a test you want to run against
other code runs in a worktree of your own.

## What you are given, and what you read first

- The diff: `git diff $(git merge-base origin/main HEAD)..HEAD`. At a concept's checkpoint, the
  diff of that family.
- The pull request's description: `gh pr view --json title,body`.
- The use cases the description names, each read completely.
- The root `AGENTS.md`, and the `AGENTS.md` of every directory the diff touches.

## What you check, in this order

1. **The use cases.** Follow `docs/usecases/README.md`, "Checking work against its use cases": each
   check the work carries is met, not met or not proven; work beyond *Where it ends*; principles
   broken on the way.
2. **The rules no gate checks.** Every rule of `AGENTS.md` and of the touched directories' files
   tagged `[partial]`, `[unchecked]` or `[owner]` that the diff can touch.
3. **The tests.** The levels of `engineering-guidelines.md` §1. For a test the description cites as
   proof, whether it can fail: it fails on the code without the change. A test that passes with and
   without the change proves nothing about it, unless the change is a pure refactoring.
4. **The design brief.** Whether the change is structural
   (`.agents/skills/design-brief/SKILL.md`). If it is: whether the description holds a brief, and
   whether the build followed it.
5. **The look.** For a screen with a mockup: the rendered mockup beside the running screen at the
   same width, region by region — spacing, proportions, sizes, focus styling, states.
6. **The text.** Three things:
   - every sentence the diff makes false. Search every tracked file (`git grep -n "<name>"`) for
     each name the diff renames or removes: an operation, a setting, a table, a message code, a
     section number. That reaches `docs/`, every `AGENTS.md`, the skills under `.agents/skills/`,
     the Markdown files at the root, the help comments of the `Makefile`, the workflows under
     `.github/`, `scripts/`, and the comments in the code;
   - every removed paragraph: was it false, or is it said elsewhere? Report each one you cannot
     answer;
   - the text the diff adds, against `docs/README.md`, "How to write".
7. **What was left behind and what was taken away.** Code, translation keys, flags and stub
   answers the change made unused and did not remove. Every deleted, skipped or weakened test,
   with the reason the description gives: the behaviour was removed or changed on purpose, or
   another test asserts the same thing. A test with neither reason is a finding.

## What you report

One table and nothing else:

| Finding | Where | Severity | Evidence |
|---|---|---|---|
| what is wrong, in one sentence | file and line, or use case and check | high, medium or low | what you read or ran that shows it |

- **High:** a rule of "Rules that do not bend", a use case check, or the safety of a person's data
  is broken.
- **Medium:** another rule or guideline is broken, or a check is not proven.
- **Low:** everything else.

Report only what you read in the code or ran. Where you could not check something, say so in a
row of its own. A review without findings reports a table with the single row "none".

## After the report

The builder fixes each finding or answers it with a reason. You read the answers once and say
which findings stay open. The description's *Review* section then states who reviewed (the model
and the date), how many findings were found and fixed, and each one left open with its reason.

You comment and decide nothing: you set no status and merge nothing (`ci-cd.md` §5).

## A pull request from a fork

CI gives a fork's run no model. The owner's worker runs this review locally on the fetched branch
(`.agents/skills/triage-contribution/SKILL.md`).
