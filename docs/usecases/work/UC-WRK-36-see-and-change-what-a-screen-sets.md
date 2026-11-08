---
id: UC-WRK-36
title: See and change what a screen sets without scrolling past the rest
context: work
actors: [PE-admin, PE-member, PE-operator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-06, P-10, P-13]
state: built
tasks: []
checked_by: [apps/webapp/build/pagekinds.test.js, apps/webapp/e2e/pagesystem.test.mjs]
---

# See and change what a screen sets without scrolling past the rest

## Goal

A settings screen shows at once which sections it has and the state of each, and a person opens the one they came for at its own address and changes it there, without scrolling past the others.

## Story

Lukas opens *Sign-in* in the administration. The list beside the content says *Passwords · 2 set here*, *Second factor · Locked*, *Sessions · Default*. He picks *Passwords*; its address is its own, the list stays beside it while he scrolls, and the section's one bar saves what he changed. On his phone the list is the page, and *Passwords* opens alone with *‹ Sign-in* to go back.

In `D1` and `D2` the person who runs the workspace is the one who uses it, and the sections are the same; in `D4`–`D7` a section the installation locked says so in the list before it is opened.

## How to check

1. Every settings screen shows its sections and each section's state in the first viewport, at 1280×800 and at 390×844.
2. A section's address opens that section; at 390×844 it opens as a detail with *Back* to the list.
3. With the seeded demo, no section is taller than two viewports at 1280×800, or four at 390×844.
4. Each section has one save model: either one save bar for its values or controls that act at once, never both.
5. At 1280×800, after scrolling to the end of a section, the column and the section list are still in view.

## Where it ends

* What each setting means and who may change it are their own use cases; this one is how a settings screen is laid out.
* The end user's vertical navigation (its content) belongs to another concept.
