---
name: ask-the-owner
description: Decide whether a question goes to the owner, and put it as a decision issue when it does. Use it when the work meets a choice on the owner's list, or when an answer from the owner arrives and has to be recorded.
---

# Ask the owner

The owner decides the items listed in `AGENTS.md`, "What you do not decide yourself". Everything
else the worker decides and writes down in the pull request.

## 1. Is it a question for the owner at all?

Ask only when both hold:

- the question is about an item of that list; and
- the work cannot go on without the answer.

Otherwise take the option that changes no rule, no gate and no use case, build on, and name the
choice in the pull request. The owner may overrule it there.

## 2. Search for an earlier answer

Search `docs/`, the `Decisions` sections of the milestone files under `docs/backlog/`, and the
closed issues labelled `decision`:

```bash
gh issue list --label decision --state closed --search "<words of the question>"
```

A question that was answered is cited, not asked again. A follow-up to an answered question names
the new fact the code showed.

## 3. Open a decision issue

Use the form `.github/ISSUE_TEMPLATE/05-decision.yml`. It asks for:

- the problem, with an example of a person meeting it;
- the proposal, and its effect on users, administrators and operators in each deployment D1 to D7
  (`docs/vision/`);
- the alternatives;
- one question the owner can answer with an option or a sentence;
- what waits for the answer.

## 4. Tell the owner once

Collect the questions of a cut or of a task's start and put them to the owner in one message, with
the list of issues. Do not ask again without a new fact.

## 5. While the answer is open

The pull request that needs the answer waits in draft. No other pull request is stacked on it.
Other tasks go on.

## 6. When the answer arrives

1. Record it on its issue at once: the option or the owner's words, the date, and where it goes.
2. The same day, open a small documentation pull request against `main` that writes the answer
   where it governs — a principle, a use case, a subject document or a milestone's `Decisions` —
   and closes the issues. One pull request per batch of answers. An answer is not written on a
   task branch.
3. An answered issue is not reopened for the same question.
