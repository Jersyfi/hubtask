---
name: decision
description: Put an open point to the owner for decision - explained in plain words, with a worked-out proposal that fits the use cases, the vision and the deployments - and then wait for his answer instead of building. Use when a readiness record has an item on the owner's list (docs/backlog/ready/README.md rule 2), when a finding during a build needs his decision, when the owner asks "what is this about", "explain the problem", "what do you propose", and before anything CLAUDE.md "What you do not decide yourself" names. The owner's own claude.ai skill "entscheidung" asks for the same form.
---

# /decision — one open point, made decidable

A question to the owner costs him time and stops a task. It is worth that only when it is on his
list and when it arrives ready to answer. Before writing one:

1. **Is it his?** Only the owner's list in [`docs/backlog/ready/README.md`](../../../docs/backlog/ready/README.md)
   rule 2. Anything else is decided by a document, a precedent, an earlier decision or the session
   itself — and written down with its reason.
2. **Was it answered before?** Search [`docs/backlog/standing-decisions.md`](../../../docs/backlog/standing-decisions.md)
   and `grep -rn` the § 8 of every record in `docs/backlog/ready/`. An answer found is cited, never
   asked again.
3. **Is the proposal true?** Every capability, lever, switch or behaviour the proposal relies on is
   checked against the code first. A proposal that promises a lever that does not exist becomes a
   false sentence in an ADR and a question that comes back.
4. **Did the matrix run?** The proposal is walked through the readiness record's § 3 (doors × states
   × kinds × D1–D7) and § 4 (the standing rules — nobody locked out). A proposal that closes one door
   and opens a lockout elsewhere is not ready to ask.

## The form — for each point

Write in the language the owner uses. As much context as needed, as little as possible.

* **What this is about** — the situation in everyday words: who is affected, what they see, in
  which deployment.
* **The problem** — why it cannot simply be built: the rule, the risk or the contradiction, with
  the document it comes from.
* **Proposal** — what to do, concretely: what a person, an administrator and an operator each meet
  afterwards; why it fits the use cases, the principles and D1–D7; what it costs (a task, a
  migration, a contract change).
* **Alternatives, and why not** — at least one, honestly weighed.
* **Until you answer** — what stays blocked, and what goes on.

## Asking

* **Batch.** All open points of a cut, or of everything that is blocked, in **one** message —
  numbered so he can answer "1 yes, 2 with the alternative". Never one by one.
* **Then stop** building what the points block. Do not build first and ask afterwards: a decision
  built first is a fait accompli, however openly the pull request reports it.
* **Record the answer the same hour**: in the readiness record's § 8 (date, his words in short),
  in `standing-decisions.md` when it holds beyond the task, in an ADR when it is architectural.
  Memory may point at it; memory never holds the only copy.
* **An unclear answer** is read back once, in one sentence, with the reading you will build — not
  turned into a new round of questions.
