---
id: UC-WRK-33
title: Tell Hubtask, my hubs and my collections apart at a glance
context: work
actors: [PE-person, PE-member, PE-child]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-10, P-13]
state: partial
tasks: [UX-01, UX-06, UX-07, F7-01, F7-06]
checked_by: [apps/webapp/e2e/looks.test.mjs, apps/webapp/src/lib/workspace/look.test.ts, packages/design-system/test/contrast.test.js, packages/design-system/test/status.test.js, packages/design-system/build/lint-status.js, packages/design-system/test/mark.test.js, apps/webapp/e2e/mark.test.mjs, apps/webapp/build/check-shell-icons.test.js]
---

# Tell Hubtask, my hubs and my collections apart at a glance

## Goal

A workspace with many hubs reads at a glance: every hub and collection has its own colour and symbol, cards stand out from the page, states are coloured surfaces, and Hubtask is recognisable by its own mark.

## Story

The person gives "House" raspberry and a house symbol, "Agency" indigo and a briefcase. The navigation, the page heads and the entries on *Today* carry them.

## How to check

1. Creating and editing a hub or collection offers a colour from the ten label colours and a symbol from the icon set; both appear in the navigation, the page head, and next to its entries in lists that mix collections.
2. Cards and panels are told apart from the page in both modes: the contrast between page and card is at least 1.18:1 and every text pair meets WCAG 2.2 AA, measured by the token test.
3. Every status is shown as a coloured surface with text or an icon, never colour alone.
4. The Hubtask mark is shown in the app bar and as the browser tab's icon, in a light and a dark form; below 24 pixels the reduced mark is used.
5. The Hubtask mark is the installed app's icon on every platform an installed app is offered for.

## Where it ends

* Colours stay the ten label tokens; no free colour values (rule 15).
* Levels have no fixed colour or symbol of their own (decision #1624, #1630).

## Today

* Check 5: met for the desktop app (Windows, Linux) and the Android app, whose icons - the launcher's included - are generated from the mark and checked in the `shell` lane.
