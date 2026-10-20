---
id: UC-WRK-26
title: Capture something from anywhere in one step
context: work
actors: [PE-person, PE-member, PE-child]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-10, P-12, P-13]
state: specified
tasks: [UX-14]
checked_by: []
---

# Capture something from anywhere in one step

## Goal

Whatever page a person is on, a thought becomes an entry in the right place with its date, person and labels before it is forgotten — one key, one line, Enter.

## Story

On the board of "Client A" the person presses `Q`, types "Call the electrician tomorrow 11am +Mia #House" and presses Enter. The entry lands in the hub's default collection with tomorrow 11:00 and Mia responsible; a note says where it went and offers to open it.

## How to check

1. `Q` (outside text fields) and the *Capture* button in the bar open the capture field on every page; on a phone the centre of the bottom bar does.
2. As the person types, the field shows what it recognised as chips — date, time, person (`+`), hub (`#`), collection (`@`), priority label (`!`), amount — and keeps the rest as the title; a chip can be removed to keep its text in the title.
3. What the capture recognises follows the target collection's settings: in a list it recognises amounts and lines, in a collection without dates it does not take a date.
4. Without a target it goes to the current collection, else the jumble; Enter creates the entry and a status message names the place with a link and an undo.
5. Dates are read in the person's language and time zone; "tomorrow" near midnight is the day after the person's today.
6. Everything the field does is possible with the keyboard alone and announced to a screen reader.

## Where it ends

* Capturing from mail or other systems is the jumble's ([UC-JUM-03](../jumble/UC-JUM-03-forward-mail-into-the-jumble.md)); capturing with AI from free text is [UC-AI-09](../suggestion/UC-AI-09-turn-text-into-entries.md).
* No speech input.

## Today

* Check 1: not met — not built yet, tracked in milestone UX.
* Check 2: not met — not built yet, tracked in milestone UX.
* Check 3: not met — not built yet, tracked in milestone UX.
* Check 4: not met — not built yet, tracked in milestone UX.
* Check 5: not met — not built yet, tracked in milestone UX.
* Check 6: not met — not built yet, tracked in milestone UX.
