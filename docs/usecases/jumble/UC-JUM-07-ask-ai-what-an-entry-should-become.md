---
id: UC-JUM-07
title: Ask AI what a jumble entry should become
context: jumble
actors: [PE-person, PE-member, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-12, P-14]
state: built
tasks: [J-06, F5-03, K-01]
checked_by: [core/application/service/jumble/Suggesting_test.go, test/integration/suggestion_test.go]
---

# Ask AI what a jumble entry should become

## Goal

In a workspace that chose to use AI, a person facing a long forwarded mail gets a proposal — a
title, notes, a due date, the steps it implies — and decides whether to take it. Nothing becomes
work without that decision, and a workspace without AI loses nothing but the button.

## Story

On an undecided entry the person presses *Ask for a suggestion*. A moment later the card shows a
proposal marked as coming from AI. They choose a collection and accept: the entry is converted with
the proposed title, notes and due date, and the proposed steps become entries under it. Or they
dismiss the proposal and convert by hand.

## How to check

1. With a provider configured and the workspace's consent given, asking queues one question and
   the proposal appears on the entry's card when it arrives, marked as a suggestion.
2. Asking changes nothing: the entry stays *Undecided* and no entry is created until somebody
   accepts.
3. Accepting converts the entry into the collection the person chooses, with the proposed title,
   notes and due date, and each proposed step as an entry under it; the conversion follows every
   check of converting by hand.
4. The proposal contains no labels.
5. In a workspace with AI off, the control is not rendered at all; a request over the API is refused
   with `ai.unavailable` — the same answer for no provider, no consent and a provider that is down.
6. Asking about an entry that is already converted or dismissed is refused with
   `jumble.entry_already_settled`.
7. Asking needs the right to write in the workspace, and a person who may only read is refused.
8. Asking twice produces two proposals, not a refusal.
9. The trail holds `jumble.suggestion_asked`, with no content of the entry.

## Where it ends

* AI never converts or dismisses on its own; a rule that asks still produces a proposal a person
  accepts ([NG-ai-required](../../vision/non-goals.md)).
* No labels from the jumble: a label belongs to a collection, and an arrival is in none yet.
* Configuring a provider and giving consent are the AI context's use cases, not this one's
  ([NG-ai-consent-by-default](../../vision/non-goals.md)).
