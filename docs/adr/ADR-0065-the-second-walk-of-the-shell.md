# ADR-0065 — Sections, and where a statement about the application lives

**Status:** accepted · **Date:** 2026-09-23 · **Accepted:** 2026-09-23

## Context

The owner walked the shell that [ADR-0063](./ADR-0063-navigation-and-the-working-surface.md) built
and named ten things. Six are defects — a band's first row drawn short, a plus outside the rail's
column, a caption drawn in a rail that cannot hold it, the connection mark with no colour in the one
state that means *everything is fine*, a stream that reads *Reconnecting…* until somebody else
writes something, and `POST /search` refusing the overview's own filter. They are issues
[#1010](https://github.com/Jersyfi/hubtask/issues/1010)–[#1012](https://github.com/Jersyfi/hubtask/issues/1012),
[#1017](https://github.com/Jersyfi/hubtask/issues/1017)–[#1019](https://github.com/Jersyfi/hubtask/issues/1019)
and need no decision: each is the code disagreeing with something already written down.

Four are not defects. They ask the product to change its mind about something ADR-0063 or
[ADR-0035](./ADR-0035-one-product-version.md) decided, and this is that change of mind.

**The administration's index.** ADR-0063 decision 7 gave the administration a navigation column of
its own *and* kept its index: "The index keeps existing as the section's own overview." With the
column listing all seventeen screens, the index is the same list drawn a second time, and arriving
at the section means arriving at a page of links to where the reader was already going.

**The reading measure.** The same decision asked every administration screen for "its content in
the reading measure the design system gives a document". Sixteen screens took that literally, as
`max-width: 60ch` around everything they draw. But these screens are not documents. The quotas
screen is a table of five columns inside a 60ch column with the rest of the region empty; the role
matrix is a grid; people, groups, service accounts, webhooks, runs and the audit trail are lists of
rows with four or five facts each. The automation screens never had the measure, which is why the
owner names them as the ones that are right.

**Your settings.** `/profile` is nine sections and 659 lines on one screen: the language and the
clock, the second factor, the sessions, the devices, the theme, the celebrations, the
notifications, the apps, the tokens. It is read by scrolling, nothing says where anything is, and
two of its lists — where somebody is signed in, and which devices hold a copy — grow without bound.
It is the same problem the administration had before decision 7, at half the size.

**The maturity notice.** ADR-0035 §2 requires the application to state its own stage while it is not
`stable`, and says it as "a banner". Since the shell wave that banner is the first thing on every
page, above the page's own head: a statement about a *release*, drawn where a statement about the
*page* belongs, pushing the working surface down on every screen. The health report the frame draws
under it has the same problem and a stronger claim to the reader's attention.

## Options

**A. Leave the three sentences and fix only the six defects.** Cheapest, and wrong for a reason the
walk makes plain: the owner is the product's first user, and each of the four is him not finding
what he needed twice in a row. An index nobody wants, a measure that wastes two thirds of a wide
screen, a settings screen read by scrolling, and a notice in the way on every page are not matters
of taste that a second opinion settles; they are the shell being harder to use than its own ADR
intended.

**B. One section pattern, applied twice, and one place for a standing statement (chosen).** The
administration already proved the pattern: while a route's area is that section, the column is the
section's list, every screen has an address, and the reader's place is the row that is current.
Nothing about it is specific to administration. What it needs to be *reusable* is exactly what this
ADR decides: what the front door of a section is, what width a section's screens have, and where a
statement about the application goes now that no page has a row to spare for one.

**C. A settings dialog.** Rejected. A modal cannot be linked to, cannot be walked by the tour, and
would make "where you are signed in" a list somebody reads inside a box over the work they were
doing. Every screen in this product has an address; a preference is not the exception.

## Decision

### 1. A section's front door is its first screen

`/administration` keeps its address — the trail on every one of its screens links to it, and so does
the account menu — and answers with the section's first screen rather than with a list. The index
screen goes.

This reverses one sentence of ADR-0063 decision 7. What replaces it: **a section that has a
navigation column needs no overview, because the column is the overview.** A reader who arrives
somewhere they did not choose is not lost — the current row says where they are, and the column says
what else there is.

### 2. A section's screens take the region; the measure belongs to prose

The reading measure is a property of **running text**, not of a screen. So:

* A paragraph, a hint, a refusal sentence — a reading measure, as they have today.
* A table, a list of rows, a matrix, a card — the region, whole.
* A form — a column wide enough for its fields and no wider, which is a measure of its own and not
  the prose one.

This reverses the second sentence of ADR-0063 decision 7. Every administration screen but the
automation ones therefore loses its `60ch` wrapper, and what needs a measure carries one where the
text is.

### 3. What is the reader's own is a section, with the same anatomy

While the resolved route's `area` is `profile`, the navigation column is Your settings' own list:

| Group | The rows, and the screen each opens |
|---|---|
| ← The workspace | back to where the reader was |
| You | *Language and clock* → How the product speaks to you · *On this device* |
| What you are told about | *Notifications* → What you are told about |
| How you get in | *Second factor* · *Signed in* → Where you are signed in · *Devices* → Devices that synchronise |
| What may act for you | *Apps* → Apps you have allowed · *Access tokens* |

**A row's word and a screen's heading may differ, and only for room.** "Where you are signed in" is
the right sentence over a table and four words too many in a column 240 px wide, where it is cut to
"Where you are sign…". So the row says *Signed in* and the screen says the sentence. Nothing else
about them may differ: a row and the screen it opens saying two different things is how a
navigation stops being trustworthy.

`/profile` is the first screen — how the product speaks to this reader: the language, the clock and
the first day of the week — and every other screen has its own address under it. `/profile/tokens`
keeps the address it has had since F4-10.

**The two lists become tables.** Where somebody is signed in and which devices hold a copy are rows
that grow without bound and carry four facts each as a sentence. As a table with a column per fact,
a reader scans one column instead of reading every row — and the newest is first, because the row
somebody looks for is the one that appeared while they were not looking.

**The notification rows become a grid.** One row per category, one column per channel, so that the
switches of every row stand in the same two places; today each row draws them where its own text
ends, and a long category name pushes them out of line with every other row.

### 4. A statement about the application is a mark in the bar

ADR-0035 §2's "a banner" becomes: **the application says what it has to say about itself from one
mark in the app bar**, beside the connection's — the pattern decision 5 of ADR-0063 already
established, for the same reason. Pressed, the mark opens what it has to say; unpressed it says only
that there is something.

| What | The mark | Pressed |
|---|---|---|
| The stage, while it is not `stable` | The quiet mark, no dot | The stage's sentence, and what it promises |
| The health report says something is wrong | The mark with a `status.warning` dot | The report's own words, per component |
| Both | The dot | Both, the report first |

The stage is not dismissible any more, and that is a simplification rather than a loss: nothing is
in the way, so nothing has to be pushed out of it. The dismiss existed because the banner took a row
of every page.

Nothing else moves into the bar. A notice about **the page** — a refused write, a check's findings —
stays where `PageHeader` draws it, because it is about the page. The rule that tells them apart:
**if it would say the same thing on every screen, it belongs to the bar.**

### 5. The account menu ends with signing out, and a version is not a suffix

Two sentences of ADR-0063 decision 6 change:

* **Sign out is the last row.** Decision 6 put *About Hubtask* under it. Signing out is the last
  thing a reader does in a session, and a row under it is a row somebody reaches past.
* **The About row does not carry the version.** `product_version` is a release version on a release
  and a build reference — `main-<40 hex>` — on everything else, and a build reference in a menu row
  is forty characters of noise where a name should be. The row names the destination; the page
  behind it quotes the version whole, which is what that page is for.

### 6. The connection mark says what the connection is, not what has arrived

A record arriving proves the stream *delivers*. It is not the definition of being connected, and
treating it as one is why an idle workspace reads *Reconnecting…* for as long as nobody writes
anything. The engine knows when the connection was accepted and must be able to say so, so
`ListenOptions` gains one callback for it, and the client's state follows the connection.

This is not a new decision — ADR-0063 decision 5's table already says what each state means. It is
written down here because the fix crosses a package boundary, and because "the ordinary case spends
no colour on itself" was implemented as *no colour at all*: the table says
`status.success.accent`, and that is what connected means.

## Consequences

* **No contract change and no migration.** One core defect (#1018) is a use case resolving what it
  already parses; everything else is the client, the design system, and one callback in
  `packages/sync-engine`.
* **`docs/design/design-system.md` gains the section anatomy** — the column, the front door,
  the widths — and ADR-0035 §2's sentence about a banner is amended by decision 4 here rather than
  edited in place, as this repository amends an accepted ADR.
* **Your settings is eight screens where it was one.** Each is smaller than the section it came
  from; none of them is new work for the server, and every read they make exists today.
* **The e2e walks change with it**: the account group's order, the administration's front door, and
  the notice mark are all asserted in `apps/webapp/e2e`.
* **What this does not do.** It does not touch who may see the administration (ADR-0061 decision 1
  stands), does not add a destination, and does not move the theme out of the device (ADR-0043
  stands — it moves to a screen of the section, still kept in the browser).

### Backlog impact

None. The ten findings are issues #1010–#1019 and are worked as defects and as this ADR's first
implementation; no milestone is cut for them.

## Notes

Related: [ADR-0063](./ADR-0063-navigation-and-the-working-surface.md) (decisions 5, 6 and 7, of
which five sentences change here), [ADR-0035](./ADR-0035-one-product-version.md) §2 (the stage and
how the application says it), [ADR-0043](./ADR-0043-theme-per-device.md) (the theme is the
device's), [ADR-0032](./ADR-0032-client-capability-matrix.md) (the areas the mobile shell switches
on, which is why Your settings is a section and not a set of end-user screens),
[ADR-0061](./ADR-0061-page-anatomy-and-the-shell.md) decision 1.
