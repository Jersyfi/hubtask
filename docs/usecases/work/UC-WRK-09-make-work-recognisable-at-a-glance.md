---
id: UC-WRK-09
title: Make work recognisable at a glance
context: work
actors: [PE-person, PE-member, PE-admin, PE-agent, PE-integrator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-08, P-12, P-13]
state: partial
tasks: [B-09, F2-10, C-06, F3-09]
checked_by: [test/integration/structure_test.go, core/domain/model/work/Label_test.go, core/domain/model/work/Cover_test.go, test/integration/media_test.go, apps/webapp/e2e/entry.test.mjs]
---

# Make work recognisable at a glance

## Goal

A collection has its own small vocabulary of coloured labels ("urgent", "waiting", "shopping"),
entries carry them, and a task can wear a cover — a colour or a picture — so that a list or a
board can be read before anything is opened.

## Story

The person who shapes the "Home" collection creates the labels it needs, each with one of the ten
colours the product offers, in both light and dark legible. On a task, the label picker toggles
them on and off. The task "Garden" gets a green cover, "Holiday" a photo; the cover sits above the
title on the entry and on the card.

## How to check

1. A person who may change the collection's shape creates, renames, recolours and deletes labels;
   the colour is one of ten named colours, never a free colour value.
2. Two labels with the same name in one collection are refused; the same name in two collections
   is two labels.
3. On an entry whose type carries labels, the picker toggles the collection's labels on and off,
   and each chip can be removed; the history records `item.label_added` and `item.label_removed`.
4. A label from another collection cannot be put on an entry; an attempt is refused with
   `labels.not_in_collection`.
5. A task offers a cover as a colour from the same ten, or as an uploaded image, and a way to
   remove it; a type without the cover capability offers none and takes no room for it.
6. After a label is deleted no entry shows it, and nothing else about those entries changes — no
   entry is rewritten and no entry's history gains a step.
7. A member without the right to change the collection's shape can use labels but is **not
   offered** to create, rename or delete them.
8. Labels are offered by name to a screen reader, not by colour alone.

## Where it ends

* No labels shared across collections or hubs — a label is part of one collection's vocabulary
  ([domain model §3.5](../../architecture/domain-model.md)); moving an entry reports the labels it
  loses ([UC-WRK-06](./UC-WRK-06-put-work-in-order-and-move-it-where-it-belongs.md)).
* No colour picker and no hex values ([ADR-0029](../../adr/ADR-0029-design-system-tokens.md)).
* No creating a label from inside the entry's picker — labels are made on the collection.
* Attachments in general belong to the media context.

## Today

* **Check 7 fails.** The labels dialog is reachable from the collection's page menu for everybody
  and draws its create, rename and delete controls without asking the role
  (`apps/webapp/src/lib/entries/LabelsDialog.svelte`, `apps/webapp/src/views/ContainerView.svelte:546`);
  the server refuses a member (`core/application/service/work/CreateLabel.go:95`).
