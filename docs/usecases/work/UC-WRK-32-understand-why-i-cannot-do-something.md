---
id: UC-WRK-32
title: Understand why I cannot do something
context: work
actors: [PE-person, PE-member, PE-admin, PE-owner, PE-auditor, PE-operator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-07, P-11, P-13]
state: specified
tasks: [UX-03, UX-04, UX-05]
checked_by: [apps/webapp/e2e/workbench-layout.test.mjs, apps/webapp/e2e/explain.test.mjs, apps/webapp/e2e/explain-settings.test.mjs, apps/webapp/e2e/workbench-fields.test.mjs, docs/evidence/UX-04-2026-10-21.md]
---

# Understand why I cannot do something

## Goal

When something cannot be done right now, the person finds out why at the place they tried, and the page around it stays as it was.

## Story

In a collection whose hub is archived, *Add an entry* is unavailable. Hovering or focusing it shows "The hub above this one is archived, so nothing here can be changed. Unarchive Old flat to work in it again."; the buttons beside it do not move.

## How to check

1. A control that is unavailable for a reason the person can resolve or wait out stays visible, can be focused, is announced as unavailable, and shows its reason on hover, on focus and on tap; showing the reason changes the size or position of nothing on the page.
2. A control the person may never use because of their role is not shown (P-05), unless the page needs it for orientation, then it shows a lock and the reason as above.
3. A form's submit button is never unavailable because a field is empty or nothing changed: pressing it marks the missing fields and moves focus to the first; a form with unsaved changes shows one bar with their count, *Discard* and *Save*.
4. When a whole area cannot be changed (an archived collection, a read-only role, a feature not in the plan) one notice at the top of the area says why and what to do; no sentence is repeated on its rows or fields.
5. A value set above the person (installation, plan, workspace) is marked next to its name with where it comes from and whether it is locked; the mark opens the value, its source and who to ask.
6. While something is being saved, the control shows progress in its own place; afterwards a short message confirms it.
7. These hold on every page of the web app, for people, administrators and operators, at every width.

## Where it ends

* Which actions are allowed is decided by the use cases and roles; this one is how a refusal is shown.

## Today

* Check 1: met by the design system's components — every control with a reason stays focusable and shows it as a bubble on hover, focus and tap without moving a box, measured in three engines (`apps/webapp/e2e/workbench-layout.test.mjs`); the screens use them through UX-04 and UX-05 (check 7).
* Check 2: met on the end-user screens and Your settings (`apps/webapp/e2e/explain.test.mjs`); not met on administration and installation, tracked in UX-05.
* Check 3: met on the end-user screens and Your settings (`explain.test.mjs`, `explain-settings.test.mjs`); not met on administration and installation, tracked in UX-05.
* Check 4: met on the end-user screens and Your settings (`explain.test.mjs`, `explain-settings.test.mjs`); not met on administration and installation, tracked in UX-05.
* Check 5: not met — not built yet, tracked in milestone UX.
* Check 6: met by the design system's components — `Button`, `IconButton`, `Checkbox` and `Switch` show progress in place without changing size (`apps/webapp/e2e/workbench-layout.test.mjs`), and the workbench's *Saving a setting* confirms with a `Toast` and *Undo*; Your settings show it in place and confirm with a toast (`explain-settings.test.mjs`); administration and installation are tracked in UX-05.
* Check 7: met for the end-user screens and Your settings at three widths (`explain.test.mjs`, `explain-settings.test.mjs`); not met for administration and installation, tracked in UX-05.
