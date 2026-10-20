---
id: UC-WRK-24
title: Change a collection's level without losing anything
context: work
actors: [PE-person, PE-owner, PE-admin, PE-agent, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-04, P-08, P-11]
state: specified
tasks: []
checked_by: []
---

# Change a collection's level without losing anything

## Goal

A hub or collection grows from a list into a project and shrinks back into a list, and whatever its entries held comes back exactly when it grows again.

## Story

The hub "Shopping" with "Wholesale" and "Supermarket" becomes the project "Summer party": the person switches the hub to *Project* and both collections follow; every item is now a task. After the party they switch back to *List*. A preview names what will no longer be shown — four work packages, eleven activities, dates on nine entries, the field "Budget" — and they confirm. A year later, back to *Project*: everything is where it was.

## How to check

1. Switching to a preset or setting that offers more takes effect at once; existing entries keep everything they hold and gain what is now offered.
2. Switching to one that offers less first shows a preview naming what will no longer be shown: deeper levels with their counts, fields with values and on how many entries, views, and rules that would pause.
3. By default the preview's choice is *Hide*: after confirming, nothing is deleted; hidden levels and values are returned by every read with a mark saying they are hidden, and writing them is refused with `capability_not_supported`.
4. An entry holding hidden content shows that it does ("3 steps hidden"), so nobody believes it lost.
5. Switching back restores the earlier view exactly: the same entries in the same places with the same values.
6. *Flatten* is offered as the second choice: every leaf becomes an entry of the top level, the parent's title becomes a label, and each entry's history records where it came from; it cannot be undone by switching back, and the preview says so.
7. A switch on a hub applies to every collection that inherits from it; a collection with its own value keeps it and the preview says which.
8. Every switch writes the audit trail with who, when and from what to what.

## Where it ends

* Switching never deletes an entry, a value, a comment or an attachment; deleting stays the lifecycle's use cases.
* No automatic switching: Hubtask may say a collection uses more than its preset, never change it on its own.

## Today

* Check 1: not met — not built yet, tracked in milestone UX.
* Check 2: not met — not built yet, tracked in milestone UX.
* Check 3: not met — not built yet, tracked in milestone UX.
* Check 4: not met — not built yet, tracked in milestone UX.
* Check 5: not met — not built yet, tracked in milestone UX.
* Check 6: not met — not built yet, tracked in milestone UX.
* Check 7: not met — not built yet, tracked in milestone UX.
* Check 8: not met — not built yet, tracked in milestone UX.
