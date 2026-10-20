---
id: UC-AI-09
title: Turn a piece of text into entries
context: suggestion
actors: [PE-person, PE-member]
deployments: [D1, D2, D3, D4, D5, D6]
serves: [P-12, P-14]
state: specified
tasks: [UX-20]
checked_by: []
---

# Turn a piece of text into entries

## Goal

A recipe, an e-mail or meeting notes pasted into the capture field become the right entries with their fields, proposed for one look and one click.

## Story

The person pastes a recipe into capture for "Groceries" and chooses *Make entries*. Five items with amounts are proposed; "milk" is already on the list and the proposal offers to raise its amount.

## How to check

1. With AI consented to, the capture field offers *Make entries* for pasted text of more than one line; without it nothing is shown.
2. The proposal lists the entries with only the fields the target collection offers, and marks entries that resemble existing ones by their title.
3. *Take all*, taking single entries, and *Dismiss* are one gesture each; nothing is created before.
4. Content of people who keep theirs out of AI is not sent.

## Where it ends

* The jumble's proposals are [UC-JUM-07](../jumble/UC-JUM-07-ask-ai-what-an-entry-should-become.md).

## Today

* Check 1: not met — not built yet, tracked in milestone UX.
* Check 2: not met — not built yet, tracked in milestone UX.
* Check 3: not met — not built yet, tracked in milestone UX.
* Check 4: not met — not built yet, tracked in milestone UX.
