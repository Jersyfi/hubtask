# ADR-0111 — Creating a hub is a page

**Status:** accepted · **Date:** 2026-11-23 · **Decided:** 2026-11-22, by the owner (#1865)

**Rule lives in:** [user-experience.md](../architecture/user-experience.md) §5.2 (*Creating a hub is a page*)

## Context

*Create hub* was the one create dialog (§5.2, "one create dialog anatomy") holding a name, a gallery
of thirty-two starts in eight families with each start's questions and plan, a folded look and
*Private*. Measured at 1512 × 907, the dialog showed 670 px of 3 189: the look began at 2 798 px
and *Private* - the most important choice at creation (ADR-0073) - at 3 092 px. With a template
chosen, *Private* was shown and ignored (#1871).

## Decision

* *Create hub* is a create page at its own address, `/hubs/new`: the hub's details in a column
  beside the choice of how it starts. *Private* stands right under the name, the look is open, and
  a summary of the chosen start says what it sets up before anything is made.
* The starts are one radio group of tiles - a gallery that is a choice, select and stay - with
  *Empty hub* first, pinned, chosen when the page opens and never filtered away; a search and a
  family filter narrow the rest.
* Every door to it navigates there, carrying a template chosen before, and *Cancel* goes back.
* On a phone the details are the page and the starts its optional second step - a full-screen
  layer over it, not a route, that back closes - so a hub made empty costs no press more.
* A collection keeps its dialog: a name, its kind and its look are *Dialog M*. Its kinds are data
  and grow with the saved presets, so its body may scroll at 1280×800 while its name and *Create*
  stay in view - the one *Dialog M* that does (decision #1865, design-system.md §11.8).

## Alternatives not taken

* **A dialog with a fold.** It keeps the anatomy, but a fold holding thirty tiles pushes the
  description and the primary a dozen screens down - the finding again.
* **A wizard** (details, then template, then review). Most hubs are made empty, so a mandatory
  second step costs every person a press; and *Private* would again sit apart from the starts it
  limits.

## Consequences

§5.2's one create anatomy has a stated exception. The page is wider than a dialog and needs the
frame's filled region from `medium` (`CreatePage`'s `isFilled`, `page.fill('medium')`), so its head
stays while the starts scroll, and below `medium` the phone's
whole screen, where the frame draws neither of its bars (`page.takeScreen`). The phone's second
step is a `Dialog` that is a step (`isStep`), held in the session history so the system's back
closes it (`Router.hold`).
