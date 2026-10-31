<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A24 — A live document's log is not compacted

**Alert:** `HubtaskDocumentLogNotCompacted` · **Severity:** ticket · **Catalogue:** A-24

## The symptom

A live document (a page, a whiteboard) keeps its updates in a log after its last snapshot. The
server asks a writer for a new snapshot every 500 updates (message `101`); the writer's editor
answers with its whole state (offline-sync.md §13). The server cannot merge on its own (SY-F).

Updates have kept landing on a log more than 2,500 updates past its snapshot - five rounds of asking
without an answer - or were refused at 5,000 (`realtime.compaction_overdue`). Opening that document
replays the whole log, so it gets slower; past the ceiling the writers' words wait in their editors
until one of them sends a snapshot. Nothing is lost or corrupted.

## Immediate action

```promql
sum by (outcome) (rate(hubtask_realtime_document_backlog_total[15m]))
```

* **`overdue` only** → writers connect and type but never answer `101`: an old or broken client, or
  an integration writing through the live protocol without compacting. Its own vendor or build is
  the matter; a current web app answers.
* **`refused` rising** → a document reached the ceiling. Any current web app opening it is asked
  for a snapshot at once and lifts the refusal; ask its people to open it once.

## Escalation

Refusals that continue while current clients have the document open point at a snapshot being
refused - look for `realtime.snapshot_too_large` (a state above 16 MB) in the server log at warn.

## Follow-up

The alert resolves once no update lands past the alarm for fifteen minutes.
