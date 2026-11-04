<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A39 — First synchronisations skip a kind

**Alert:** `HubtaskSyncKindSkipped` · **Severity:** ticket · **Catalogue:** A-39

## The symptom

Within the last hour an initial synchronisation could not read one kind of object - the labels,
the comments, the pages - and went on without it (`hubtask_sync_kinds_skipped_total`). Devices
that synchronised for the first time hold everything else and are told which kinds they lack
(`skipped_kinds` on the pull's answer). Each later pull from such a device, once its delta is
current, walks the skipped kinds again; the device is complete as soon as the read succeeds.

## Immediate action

The server logs each skip at warn: `an initial synchronisation skipped a kind whose read failed`,
with the kind and the failure's code - never content.

* **`postgres.query_failed` or `dependency_unavailable`** → the database was unwell while the walk
  read; the next pull reads the kind again by itself. Nothing to do once the database recovers.
* **Any other code, repeating for one kind** → a row the kind's reader cannot turn into a record
  (a value the domain refuses), or a defect in the reader. Find the workspace from the request log
  of the failing pulls, read the kind's rows there, repair the row or open an issue with the code.

## Afterwards

The alert clears an hour after the last skip. Devices that skipped the kind catch it up on their
next pull; none needs to be reset.
