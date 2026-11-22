---
id: UC-FLW-06
title: See where an entry can go next and how its workflow looks
context: workflow
actors: [PE-member, PE-guest, PE-admin, PE-integrator]
deployments: [D3, D4, D5, D6, D7]
serves: [P-05, P-08, P-12, P-13]
state: partial
tasks: []
checked_by: [core/domain/model/work/Workflow_test.go, core/application/service/work/EntryStepsPicture_test.go, apps/webapp/src/lib/entries/entrysteps.test.ts, apps/webapp/e2e/entryhead.test.mjs]
---

# See where an entry can go next and how its workflow looks

## Goal

A person looking at an entry knows where it can go from here, which steps are theirs to take,
which are somebody else's and whose, and how the entry gets to done — without asking the person
who designed the workflow.

## Story

Tom opens a story in "In review". The status button opens "from *In review* on": *Done*, which
asks for a resolution, and *Back to work*. Below them, as one line of information and not as a
button, "*Reject* → Rejected: only the product owner, Clara Neumann". He wonders why the story
needs a review at all and opens *view workflow*: the statuses in their categories and the steps
between them, drawn read-only; the designer is not offered to him. Clara, who shapes the hub,
sees *open in designer* on the same picture. In a collection without a workflow the menu shows
to do, in progress and done, and the main button is *Done*, which completes the entry.

## How to check

1. The status control of an entry offers each step the person may take with the status it leads
   to and what it asks — the fields to fill and whether an approval is opened — and names, beside
   them as information and not as a control, each step from the status the person may not take
   with its reason (limited to roles, people or groups; or a condition that does not hold) and
   who may: the role, the person, the group.
2. A step named as one the person may not take is refused through every door, as in UC-FLW-03.
3. The entry shows how done is reached: the fewest steps from its status into a done status.
4. Everyone who may read the entry opens its workflow's picture read-only — the statuses with
   their categories, the steps between them, which are limited and which wait for an approval;
   *open in designer* is shown only to a person who may change the structure where the entry is.
5. Where no workflow runs, the control shows the entry's level: in a list without columns (L0)
   open and done; on a board (L1) the board's columns as the states, the done column marked as
   completing; its main action completes the entry (owner's decision of 2026-11-12).
6. REST, MCP and `hubctl item steps` answer the same: the reasons as codes with their parameters,
   never as sentences.

## Where it ends

* The picture shows no step's conditions, checks or rules; those stay in the designer.
* No request for a step to somebody who may take it; a comment mentioning them does that.
* The route to done is the shortest by steps, not by who may take them.

## Today

* Check 5: not met where the Workflows module is off — the steps are read from the workflows module, so a board's entry shows open and done instead of the board's columns; tracked in #1864.
* Check 6: not proven — the steps' door is one use case behind REST, MCP and `hubctl item steps`, and no test compares their answers.
