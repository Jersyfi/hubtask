---
id: UC-WRK-25
title: Keep a simple list
context: work
actors: [PE-person, PE-member, PE-child]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-10, P-13]
state: verified
tasks: [UX-12]
checked_by: [apps/webapp/src/lib/entries/capture.test.ts, apps/webapp/src/lib/entries/simplelist.test.ts, apps/webapp/src/lib/entries/swipe.test.ts, apps/webapp/e2e/simplelist.test.mjs, apps/webapp/e2e/figure.test.mjs, core/domain/model/work/Amount_test.go, core/domain/model/work/Categories_test.go, core/domain/model/work/CompletionCount_test.go, core/application/service/work/AmountGate_test.go, core/application/service/work/StructureCategories_test.go, core/application/service/work/CompletionCount_test.go, test/integration/item_amount_test.go, test/integration/completion_count_test.go, test/integration/structure_categories_test.go, test/integration/list_devices_test.go]
---

# Keep a simple list

## Goal

A shopping or packing list is quicker in Hubtask than on paper: type it, tick it, and bring back what is needed again with one tap.

## Story

The family's "Groceries" list is open on two phones in the shop. One person ticks "milk"; it moves to *In the trolley* on both. Next week "milk" is under *Recently* and one tap puts it back on the list, with its amount.

## How to check

1. In a collection whose levels are one, typing several lines into the capture field creates one entry per line.
2. A leading amount ("2 milk", "1 kg apples") is stored as the entry's amount, not as part of the title.
3. A ticked entry leaves the open list for a section *Done* (named by the collection's settings, e.g. *In the trolley*) and appears there on every device of the people on the list.
4. Ticked entries are offered under *Recently*, most often used first; choosing one reopens it (the same entry, [UC-WRK-04](./UC-WRK-04-tick-work-off-and-take-it-back.md)) instead of creating a new one.
5. Entries may be grouped by a label the collection uses as its category (aisle), and the list shows them in that order.
6. On a phone, an entry is ticked with one tap on a target of at least 44 by 44 points and with a swipe; the list works offline and catches up ([UC-SYN](../sync/)).
7. The head of the list shows how many entries are open and, if the collection sums a number field, the sum; it shows no percentage.

## Where it ends

* No prices from shops, no product catalogue, no barcode scanning.
* Sharing the list is [UC-WRK-21](./UC-WRK-21-share-a-hub-a-collection-or-one-entry.md).

