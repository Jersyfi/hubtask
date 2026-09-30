---
id: UC-MED-02
title: Give an entry a cover
context: media
actors: [PE-person, PE-member, PE-child]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-12, P-13]
state: built
tasks: [C-06, F3-09, F10-12]
checked_by: [core/application/service/work/SetCover_test.go, test/integration/media_acceptance_test.go]
---

# Give an entry a cover

## Goal

A person makes an entry recognisable at a glance on a board — with a colour or a picture — and
only where the kind of entry has a cover at all.

## Story

On a task the person opens *Cover* and either picks one of the colours or uploads a photo. The
card on the board shows it. Later they replace the photo; the old one is no longer shown anywhere
and is cleaned up once nothing else uses it. They remove the cover and the card is plain again.

## How to check

1. A cover is either a colour from the design system's set or an image, never both; any other
   colour is refused.
2. An image cover must be an upload confirmed for use as a cover in this workspace: an unconfirmed
   one is refused with `media.not_ready`, one uploaded as an attachment with
   `media.usage_mismatch`.
3. Only types whose profile carries a cover — a task by default, not a work package or an
   activity — offer and accept one; the screen shows no cover control for the others.
4. Setting a cover needs the right to edit the entry; the cover image is readable by everybody who
   may read the entry, and by nobody else.
5. Replacing a cover is the same act again; the replaced image loses its reference and is cleaned
   up once nothing refers to it.
6. Removing a cover leaves the entry without one; removing a cover that is not there changes
   nothing.
7. The cover travels to devices and merges like the entry's other single fields.
8. The screen tells the person the upload limit before the upload, and refuses a larger file
   before sending it.

## Where it ends

* No cropping, filters or resizing in the product.
* No cover on a hub or a collection.
