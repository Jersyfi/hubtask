# ADR-0115 — Help text in five tiers, the why on demand

**Status:** accepted · **Date:** 2026-11-24 · **Decided:** 2026-11-24, by the owner

**Rule lives in:** [design-system.md](../design/design-system.md) §4, "Help text" and case 2 of
"A control that cannot be used now", and §6, "A scroll shade"

## Context

The design system had two places for help: a hint that is always visible, and nothing. Every why,
every background and every example therefore went into the hint. Hints grew to four lines on a
phone, longer than the field they explain, and an introduction stood above a second introduction.
A menu item's reason was a second line under the item, although §4 already said that a reason is a
bubble at the control; in a long menu those lines ran over the next item. The scroll shades of a
menu showed at both edges whatever was left to scroll, and lightened a dark surface into a bar.

## Decision

* **Five tiers**, each text in exactly one: the name, the hint (what is needed now), *more about*
  (the why, behind an ⓘ beside the label or the heading, `HelpTip`), the reason (the `_Reason`
  bubble) and the tooltip (a name, never help).
* ***More about* opens on a click or a tap, never on a hover**, and stays a popover on a phone.
* **A locked menu item keeps its one line** with an ⓘ at its end; its reason is the bubble every
  other control uses.
* **The owner's three conditions:** a field's or a setting's description stays visible and only
  the why moves behind the ⓘ; no help text, description or reason is cut with an ellipsis; a scroll
  shade stays inside its surface's rounded corners.
* **A scroll shade** shows only at an edge with more beyond it, over the rows, and darkens the
  surface in both themes.

## Consequences

* A hint can be one line again without losing what explained it.
* A reader on touch learns of a reason or of more help by the ⓘ, not by hovering.
* What is no longer visible at a glance is read less; that is why the hint keeps everything a
  person needs to act, and the description never moves.
* The existing texts are rewritten into the tiers in a pass of their own (design-system.md §9).
