# Module 3 — Knowledge: pages, wiki and notes (`UC-DOC`)

> **Background** — the concept of 2026-10-17; where it differs from [`../products.md`](../products.md),
> that file wins ([`README.md`](./README.md)).

## 1. Goal

The things a team needs to *know* live beside the things it needs to *do*: a hub has its pages — a
handbook, meeting notes, a specification — written together at the same time, linking to entries
and turning a checklist line into a task. A person keeps private notes the same way. Everything is
in the same backup, trail, search and export.

## 2. Parity

| Capability (market) | Proposed |
|---|---|
| Spaces / page trees | A hub *is* a space: its pages form a tree under it; private hubs give private spaces (ADR-0073 holds for pages unchanged) |
| Block editor (headings, lists, tables, code, callouts, toggles, embeds, images) | A block editor whose document is stored as a structured document (blocks with a schema), Markdown export lossless for the common subset |
| Real-time co-editing, presence, cursors | A CRDT per page synchronised over the existing stream; offline edits merge by the CRDT (sync engine) — **new dependency, decision D-04** |
| Inline comments, resolved threads | Comments anchored to a range; same notification paths |
| Mentions of people, entries, pages; backlinks | `@person`, `#entry`, `[[page]]`; backlinks listed on each target |
| Task lists that become tasks | A checklist line converts into a `TASK` in a chosen collection and stays linked (its tick mirrors completion) |
| Page templates (meeting notes, retrospective, decision record, how-to) | Shipped templates + workspace templates, like item templates |
| Version history, compare, restore | Snapshots per edit session; compare two; restore as a new version (P-03) |
| Page permissions, restrictions | Inherit the hub's memberships; a page may be restricted to fewer (never widened, like private hubs) |
| Public / guest sharing of a page | Read-only share by `ITEM`-like membership for a `GUEST`; a public link only if the workspace allows (lock-able per plan) |
| Embedded live views (a saved view, a chart) | Embed blocks for saved views and dashboard widgets |
| Databases inside docs | Not as a second data model: an embedded saved view over records/entries (`NG-spreadsheet`) |
| Personal quick notes, daily note | A personal space per account (a private hub created on first use) with a "today" note |
| Export: PDF, Markdown, HTML; import from other wikis | Markdown/HTML export; import from Markdown folders and common wiki exports (importer context) |
| Search across pages and entries | The existing search extended to pages (full text, Postgres) |
| AI: summarise, draft, ask the wiki | Suggestion context: optional, consented (P-14) |

## 3. Use cases (titles)

| ID | Title |
|---|---|
| DOC-01 | Write a page in a hub and find it again |
| DOC-02 | Write a page together at the same time |
| DOC-03 | Turn a line of a page into a task and keep them linked |
| DOC-04 | Link pages and entries and see what links here |
| DOC-05 | Start a page from a template |
| DOC-06 | See how a page changed and bring an older version back |
| DOC-07 | Keep notes only I can read |
| DOC-08 | Share a page with someone outside, read-only |
| DOC-09 | Bring a wiki in from elsewhere, and take it out again |
| DOC-10 | Embed a live view of work in a page |

## 4. Principles, rules, where it ends

P-01, P-03 (version history, trash), P-08 (pages through REST and MCP — an agent can read and
write pages), P-10 (D1: a notes app that runs on a small box), P-13 (editor keyboard-complete).
Rule 10: page content is user content — never in logs, metrics, traces or the trail (the trail
records "page edited", not the text). Retention, legal hold and erasure apply to pages
(`data-retention.md`, `data-catalog.md` entries).

Ends: no desktop publishing; no public website builder; no spreadsheet blocks with formulas.
