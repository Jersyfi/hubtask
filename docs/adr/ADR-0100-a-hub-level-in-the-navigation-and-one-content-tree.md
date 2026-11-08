# ADR-0100 — A hub level in the navigation, and one content tree

**Status:** accepted · **Date:** 2026-11-07 · **Decided:** 2026-11-07, by the owner (approval of
the navigation concept *Kompass*, version 3, [`backlog/navigation/README.md`](../backlog/navigation/README.md) §12)

**Rule lives in:** [user-experience.md](../architecture/user-experience.md) §1 (UX-1) and §2.4,
[design-system.md](../design/design-system.md) §11.3

## Context

UX-1 gave every product a workspace-wide home and, inside a hub, one tab in a fixed strip; the
column's tree mirrored the tabs as rows. On `main` a hub's *Pages* stood on the screen three times
at once, up to five navigation levels lay on top of each other (bands, mirrored rows, the strip, a
tab's second strip, a product's own tabs), the wiki drew a second tree beside the page, whiteboards
were a flat list on a tab of their own, and a phone scrolled two tab rows. The products had been
laid out side by side, by product name, without a model of what a hub holds.

The code inventory showed three kinds of thing in a hub: work tools that compute over the hub
(queues, plan, inventory, time, goals), lists of work (collections), and things people write and
file (pages, boards). Other suites converge on the same split: written content in one mixed tree,
live queues with counts in the sidebar, computing tools as one entry that opens a page with its own
tabs.

## Decision

1. **A hub is a level of the sidebar, not a strip of tabs.** Opening anything of a hub shows the
   hub's level: its head, *Overview* and its tools in a fixed order, its collections, its contents.
   The hub's overview has no tabs. A product keeps one home across the hubs.
2. **One view bar per object.** A collection, a tool and a product home have one row of views
   under the head; there is never a second strip.
3. **Whiteboards join the pages in one content tree.** A board stands at the top or under a page,
   has no children, and keeps the hub's access. This needs a board's parent page and order, and a
   move — additive fields and one operation.
4. **Tools have a fixed order and place.** They are switched off at the hub, not hidden by a
   person: recognising a hub's shape in every hub goes before personal arrangement.
5. **Another hub only through a pre-check.** Dragging orders within a hub; a collection reaches
   another hub through *Move to…*, which asks the server what would stop the move before it is
   made.

This supersedes the tab strip of `user-experience.md` §2.4 and the mirrored rows of §2.4's last
item; the rest of UX-1 (one home per product, never in a page action or an overflow) stands.

## Consequences

* `PlaceTabs` and `placeTabs.ts` go; `ViewBar` and the sidebar's level components replace them;
  the former hub-tab addresses are kept as moved addresses.
* New contract: stars, a board's place and move, the move pre-check, the hub's queues with their
  counts, sidebar preferences (`backlog/navigation/README.md` §7).
* Pages and boards still cannot move to another hub, and hubs have no folders; each would be its
  own decision with a server change.
