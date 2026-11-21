---
name: build-concept
description: Build a whole concept on one branch and in one pull request, on the owner's word. Use it when the owner says to build a concept completely and bring it to main, and when resuming such a build.
---

# Build a concept whole

A concept built whole stays on one branch and enters `main` as one pull request, on the owner's
word at the end. The bar is a task's bar. `docs/backlog/README.md`, "A concept built whole", holds
the rules; this page holds the order of the work.

## Before the build

1. Read the concept completely, and its mockups where it has them.
2. Run the cut checklist of `docs/backlog/README.md` (items 1–7, 9, 10) and tell the owner what is
   open.
3. Open one draft pull request. Write the design brief into it
   (`.agents/skills/design-brief/SKILL.md`): a concept is a structural change.

## When one session steers workers

One worker can build a concept alone. A concept too big for one context is built by workers who
commit one after another to the one branch (`docs/backlog/README.md`, "A concept built whole").
When one session steers them, it runs like this, and builds nothing itself:

1. The steering session gives each stage to one fresh worker, one worker at a time, with the
   concept's sections for that stage and what "done" means for it.
2. When the worker reports, the steering session runs the stage's checks itself. It does not take
   a green result from the report.
3. It starts the independent review on the stage's diff (`.agents/skills/review-change/SKILL.md`).
4. A stage is closed when the checks and the review are both clean. Findings are fixed before the
   next stage starts.
5. It pushes and names the next stage on the draft pull request before its own context passes
   about 250,000 tokens.

## The order for a concept with screens

1. **The inventory.** Every view, menu, dialog and deeper page in scope, each mapped to a mockup
   scene or to a rule. Capture each one on `main` before anything changes.
2. **The design-system foundation**, then **the shared server capabilities**.
3. **Families of screens**, each family built whole.
4. **A checkpoint after every family:**
   - `make check`;
   - the family's walks: `make e2e SUITES="<suites>"`;
   - the independent review of the family's diff;
   - one CI run on the pushed branch, started by hand where `ci.yml` offers a manual start. The
     pull request stays a draft.

   A family is closed when its checkpoint is green.
5. **A coherence pass.** One fresh reviewer walks the whole inventory across the families.
6. **The presentation.** The result runs locally with demo content (`make demo`) and is shown to
   the owner before the pull request leaves draft.

A concept without screens follows the same shape: its stages take the place of the families.

## The close

Follow `.agents/skills/finish-pull-request/SKILL.md`. The description names every part of the
concept as built, built differently, or left out and why. The merge waits for the owner's word.
