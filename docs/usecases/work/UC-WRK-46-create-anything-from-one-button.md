---
id: UC-WRK-46
title: Create anything from one button
context: work
actors: [PE-person, PE-member, PE-admin, PE-owner]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-13, P-17]
state: specified
tasks: []
checked_by: [apps/webapp/src/lib/create/kinds.test.ts, apps/webapp/src/lib/shortcuts.test.ts]
---

# Create anything from one button

## Goal

A person who wants to make something - an entry, a page, a whiteboard, a goal, a dashboard, an inventory item,
time or a collection - finds it behind one button, in the place they stand, and never sees a kind they may not
make there.

## Story

In Produktentwicklung the person presses *Erstellen*. On top is the line that makes an entry from a sentence;
below it *Aufgabe* and *Seite*, made last, and every kind the hub allows. They press `S`: an empty page opens in
the hub, ready to be written. On the phone they hold the ＋ and a new entry starts in the same place.

## How to check

1. *Erstellen* in the bar, `C` or `Q` outside text fields and the phone's ＋ open one menu on every page; its first
   line is the capture field ([UC-WRK-26](./UC-WRK-26-capture-something-from-anywhere.md)) and the entry it makes
   is made there (work surface C-1, C-5).
2. Below the line stand the kinds made last on this device, the kinds the place allows, and a request to a
   department through the person's portals; a kind whose module is off or that the person's role may not make
   there is absent, not grey (P-05, P-17).
3. The place is preset from where the person stands, else from where they made something last; it is a chip they
   change, and the kinds follow the place chosen (C-2).
4. A page, a whiteboard and a dashboard are made at once and open empty; an entry, a goal, an inventory item, time
   and a collection open their own dialog (C-3).
5. A kind's letter (E, S, W, Z, D, K, T) makes it while the menu is open; on a phone holding the ＋ - or the
   context-menu key on it - makes the kind made last, where the place allows it (C-5).
6. From a template and an import stand at the menu's foot (C-4).

## Where it ends

* What capture reads from a sentence stays [UC-WRK-26](./UC-WRK-26-capture-something-from-anywhere.md).
* A department's whole directory of portals is the service desk's ([UC-SVC-05](../servicedesk/UC-SVC-05-ask-the-service-desk-for-help.md)).
* No kind is made outside a place the person may write in.

## Today

* Check 1: not met — built, no walk opens *Erstellen* from the bar, `C`, `Q` and the phone's ＋ yet.
* Check 3: not met — the place chip is built, no walk changes it yet.
* Check 4: not met — built, no walk makes a page at once or opens a kind's dialog yet.
* Check 5: not met — the letters are proven, no walk holds the phone's ＋ yet.
* Check 6: not met — built, no walk opens a template or an import from the foot yet.
