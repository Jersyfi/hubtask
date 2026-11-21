---
name: triage-contribution
description: Handle a pull request or an issue from somebody outside — say what happens to it, complete its description, run the independent review locally, and bring the result to the owner. Use it when a pull request arrives from a fork, and at a cut when findings are labelled for newcomers.
---

# Triage a contribution

A contribution is held to the same bar as every other change, and its author is never stopped by
the project's own paperwork.

## What is welcome

- **Without asking first:** a bug fix, a translation, documentation, a test.
- **Starting as an issue:** a feature (the feature form), and anything on the owner's list
  (`AGENTS.md`, "What you do not decide yourself"). When such a pull request arrives without an
  issue, say so in a comment, link the form, and bring it to the owner.
- **Work done with an AI tool** is welcome on the same terms: the contributor answers for it, and
  it passes the same gates.

## A pull request from a fork

1. **Answer.** Thank the author and say what happens next: a worker triages it and runs a review,
   the owner merges. Promise no time.
2. **Read the diff before running anything from it.** A fork's code runs on your machine with your
   access.
3. **The description.** When the description gate refuses it, complete the description yourself
   from `.github/PULL_REQUEST_TEMPLATE.md` and check it with
   `make gate-pr BODY=<file> TITLE="<title>"`. Do not send the author back to the template.
4. **The review.** CI gives a fork's run no secrets, so no model reviews it there. Fetch the
   contribution into a branch and a worktree of its own, so that the checkout you work in stays
   as it is:

   ```bash
   git fetch origin pull/<number>/head:contribution-<number>
   git worktree add ../contribution-<number> contribution-<number>
   ```

   `gh pr checkout <number>` alone is not that: it switches the checkout you are in. In the
   worktree, run `make check` and start the independent review
   (`.agents/skills/review-change/SKILL.md`). When the pull request is merged or closed, remove
   the worktree with `git worktree remove ../contribution-<number>`.
5. **The findings.** Post them as one comment the author can act on.
6. **CI.** The same required check holds. A first-time contributor's run starts after it is
   approved on GitHub.
7. **The owner.** Bring the *Use cases* table, the *Premises* and the review's summary to the
   owner. The merge is the owner's word.

## Findings for newcomers

At each cut, an open `finding` that is bounded and needs no decision gets the label
`good first issue`.

## What a contributor is not asked for

No contributor agreement and no sign-off: what is submitted is licensed under Apache-2.0
(`CONTRIBUTING.md`, "Licence").
