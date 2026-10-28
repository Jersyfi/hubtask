---
id: FLW-03
title: Move an entry on, and be told what it still needs
context: workflow
actors: [PE-member, PE-guest, PE-agent, PE-integrator, PE-scripter]
deployments: [D3, D4, D5, D6, D7]
serves: [P-05, P-08, P-11, P-12, P-13]
state: specified
tasks: []
checked_by: []
---

# Move an entry on, and be told what it still needs

## Goal

A person moves their work to its next status with one action, sees only the steps they may take,
and when a step needs something first, is told what — in words, never with a code.

## Story

Tom opens a bug in "In review". The status button offers "Done" and "Back to work" — not "Reject",
which only the product owner may take. He chooses "Done"; the step asks for a *Resolution* and a
comment, which he gives, and the bug is completed. On the board he drags another card into "Done";
it asks the same. His phone, offline, queues a move that the server later refuses because the
entry was reassigned meanwhile; the app tells him why and leaves the card where the server has it.

## How to check

1. An entry shows its status and offers exactly the steps the person may take from it, by their
   names.
2. Taking a step by the status control, by dragging on the board, by the keyboard, by the bulk
   action or through any door applies the same rules.
3. A step that needs fields asks for them before it is taken; a refusal names, per field or
   condition, what is missing, in the person's language.
4. A completed or reopened entry through "complete" on any door — including a calendar app — takes
   the step into or out of a done status the person may take, or is refused with the reason.
5. An offline move refused on the server comes back to the device with the reason, and the entry
   shows the server's status.
6. Each step taken is in the entry's history with who took it and from which status.
7. A bulk move takes each entry through its own rules and reports, per entry, the ones refused.

## Where it ends

* No step is taken by time passing; a rule does that (automation).
* No undo of a step beyond taking a step back that the workflow offers.

## Today

* Check 1: not met — the suite plan, S-07.
* Check 2: not met — the suite plan, S-07.
* Check 3: not met — the suite plan, S-07.
* Check 4: not met — the suite plan, S-07.
* Check 5: not met — the suite plan, S-07.
* Check 6: not met — the suite plan, S-07.
* Check 7: not met — the suite plan, S-07.
