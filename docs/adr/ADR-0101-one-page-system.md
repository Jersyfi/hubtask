# ADR-0101 — One page system: page kinds, two levels, the dialog ladder

**Status:** accepted · **Date:** 2026-11-08 · **Decided:** 2026-11-07, by the owner (the concept
[page-system](../backlog/page-system/README.md) released "as proposed", with D-1…D-4 as proposed)

**Rule lives in:** [design-system.md](../design/design-system.md) §6 (the surfaces and levels),
§11.1 (every screen one page kind), §11.8 (settings one section per address, the dialog-or-page
ladder); [user-experience.md](../architecture/user-experience.md) §5, §8.10;
[api-guidelines.md](../architecture/api-guidelines.md) §4 (`page.total`)

## Context

The owner (2026-11-07): across the application the screens were built differently. The survey
(page-system §2) found nine shapes a page took; forms open above a table; levels nested (a section
card holding a filter bar holding raised cards); a section list that became a sideways strip on a
phone, with no way back once inside; long lists nothing narrowed. Each screen had been built on its
own; together they set no standard a person could learn once.

## Decision

1. **Every screen is one page kind** — `ListPage`, `RecordPage`, `SettingsLayout`, `EditorPage`,
   `OverviewPage`, `LogPage`, `CreatePage`, `WorkPage`, `DetailPage`, `StandalonePage` — and the
   kind fixes which composition sits in which slot. A client fills the kinds; it draws no page
   shape of its own. Gates hold it on every routed view (`apps/webapp/build/pagekinds.test.js`).
2. **Two levels of containment**: the canvas and one region per thing the page is about; no
   shadow without movement or an overlay, and at most one focal card.
3. **One ladder decides where a task opens**: in place, popover, Dialog S, M or L, a create page,
   a record page — the first rung that fits what the task holds. A dialog holds no table, no tabs,
   no second dialog except *Discard changes?* and at most one fold
   (`apps/webapp/build/dialogs.test.js`).
4. The owner's decisions of 2026-11-07:
   * **D-1** — no page numbers: the cursor and *Show more* stay, and a list's foot says *Showing
     50 of 1 240* where the endpoint answers the optional `page.total`, which only lists that count
     cheaply give.
   * **D-2** — settings are one section per address everywhere; the section list says each one's
     state.
   * **D-3** — an administered object opens as a dialog or its record page, never in a side panel
     beside its list; the entry's panel beside a collection stays as built.
   * **D-4** — at the tablet's portrait width settings and editors drill in like a phone rather
     than show two narrow panes.
5. **The design system follows the path of the leading work-management suite's design system**:
   a small set of page templates over a shared component layer, density and levels set by tokens,
   the same patterns across every product of the suite — so that a person who has learnt one
   screen has learnt the others.

## Alternatives

* **A side panel for administered objects**, as in the first round: it keeps the list in view,
  but splits a narrow screen in two, nests a form beside a table, and gives a phone nothing to
  fall back to. Withdrawn (D-3); it stays only for the entry, where a person chooses it.
* **Page numbers for administration tables**: familiar, but they need offset reads, which the API
  refuses for the stability of a page boundary under concurrent writes, and they change the
  owner's rule of *Show more* only. Not taken (D-1); the total gives the trust a pager gives.
* **Per-screen judgement, held by review**: what produced the nine shapes. Not taken; the kinds
  and the ladder are gated.

## Consequences

* Every administration, installation and settings screen was migrated onto the kinds; new screens
  start from one, and the gates fail on a view without a kind or a dialog off the ladder.
* `PageInfo` carries an optional `total`; `GET /groups`, `GET /memberships` and `GET /templates`
  answer it, and a list that filters rows by rights after the read, reads across workspaces or
  grows without bound leaves it out.
* A change to a page kind changes every screen of that kind at once, which is the point and the
  cost: a kind's change is reviewed against all of its screens.
