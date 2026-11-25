# ADR-0116 — A surface with a tip is placed by measurement

**Status:** proposed · **Date:** 2026-11-25 · **Supersedes in part:** [ADR-0039](./ADR-0039-overlay-positioning.md), for the reason bubble, `Tooltip` and `HelpTip` only

**Rule lives in:** [design-system.md](../design/design-system.md) §6, "Overlays are positioned by CSS", and §4, "Where a bubble stands"

## Context

[ADR-0039](./ADR-0039-overlay-positioning.md) places every overlay by CSS anchor positioning in
`src/anchor.ts`, and §6 said that no component measures anything. The owner approved a tip on the
reason bubble, the tooltip and the help tip (2026-11-25): a tip points at its element after any
flip or shift, never enters the surface's rounded corner, and where the element is so near the
window's edge that the tip could not reach it, the surface comes nearer that edge, down to
`sp-100`.

CSS anchor positioning cannot do that. It does not tell a surface which of its fallbacks it took,
so a tip does not know which edge it stands on; and its shift into the window has no rule for
coming nearer the edge only so far as a tip needs. The bubble's old trouble came from the same
place: a surface sized by the room beside its anchor never overflowed, so the engine never tried
its other sides.

## Decision

* **A surface with a tip is placed by measurement.** `anchorTo`'s `tip` option measures the
  anchor and the surface after layout; `placeWithTip`, a pure function, picks the side, the shift
  and the tip's place; the answer is written into the surface's rule in the constructed
  stylesheet, never as an inline style. It is placed again on a scroll, a resize and a change of
  size while it is open.
* **Every other overlay stays on CSS anchor positioning**, as ADR-0039 decided: menus, popovers
  without a tip, the tour, the toolbar.
* **Still in one place.** Only `anchor.ts` measures; no component measures where it is drawn.

## Consequences

* A tip's surface can lag a scroll by one frame; a menu cannot. The tip surfaces are small and
  short-lived, and the bubble closes on a scroll of what it describes.
* The placement rules are tested without a browser (`test/anchor.test.js`), and the walks with
  classic scrollbars shown (`bubbleplace`, `workbench-bubble`) measure the drawn tip.
* No dependency: the placement is about a hundred lines, Floating UI's flip, shift and arrow as an
  algorithm.
