# ADR-0039 — Overlays are positioned by CSS, with one fallback we own

**Status:** accepted · **Date:** 2026-09-02

## Context

`design-system.md` §4 lands `Tooltip`, `Menu`, `Popover`, `Dialog` and `Toast` together in wave 1b
(`F1-06`). Four of the five have to sit against something: a menu under its button, a tooltip
above its control, a popover beside the thing it explains — and each of them has to stay on screen
when the anchor is near an edge, which means flipping to the other side, sliding along the edge, or
shrinking. That is one problem with one answer, and `design-system.md` §9 named it as a gap rather
than letting whoever writes `Tooltip` first answer it for the other four.

It is named there for a second reason. One of the answers is a library, and a library is a
supplier — `CLAUDE.md`'s "what you do not decide yourself" — so it is not a component author's
call. This ADR is where that call is made.

`F1-13` builds the layering scale that goes with it: `primitive.layer` in `tokens.json` says what
paints over what, and `src/layers.ts` says what `Escape` reaches. Neither of those answers *where*
an overlay is drawn, which is what is left.

## The constraint that decides it

Anything chosen here runs under [ADR-0028](./ADR-0028-embedded-web-ui.md)'s content security
policy: `style-src 'self'`, with no `'unsafe-inline'`. A positioner that writes
`element.style.top = …` is writing an inline style, and **the CSP blocks it**. This is not a
detail — it disqualifies the obvious implementation of every JavaScript option, and any of them
has to write into a CSS custom property registered on a stylesheet rule, or into a
`CSSStyleSheet` object, instead.

The second constraint is that this repository counts its dependencies. `ADR-0037` rejected
Storybook at +262 packages for a tool no user runs; a positioner ships to every user, which makes
the bar higher rather than lower.

## Options

Support measured on 2026-09-02 from `caniuse` (`css-anchor-positioning`), quoted rather than
remembered:

```
Chrome 125+   Edge 125+   Firefox 147+   Safari 26+   iOS Safari 26+   Samsung 27+
global usage with full support: 84.12 %
```

All four engines ship it. The remaining ~16 % is people on versions released before their
browser's support landed, which for Firefox and Safari is recent.

**A. CSS Anchor Positioning only.** `anchor-name` on the trigger, `position-anchor` and
`position-area` on the overlay, `position-try-fallbacks` for the flip. No JavaScript, no
dependency, and the flip happens during layout rather than a frame later — which is the difference
between a menu that opens in the right place and one that visibly jumps.

Rejected on its own, because of what an unsupported browser does. `anchor()` in a browser that
does not know it makes the declaration invalid, so the overlay falls back to its static position:
top-left of the containing block, not near the anchor at all. That is not degradation, it is a
broken menu for one visitor in six.

**B. `@floating-ui/dom`.** The library everybody uses, and it is genuinely good: flip, shift,
size, arrow and virtual anchors, tested against every engine.

Rejected on two counts. It is a supplier for a problem the platform now solves — adopting a
dependency in the year its replacement reached every engine is adopting it for its whole life. And
its documented usage assigns `Object.assign(element.style, …)`, which ADR-0028's CSP refuses; using
it here means using it against its own grain and owning that difference forever.

**C. One positioner of our own, for everyone.** No dependency and one code path, so there is no
"works in Chrome, untested in Firefox" seam.

Rejected because it is the option that spends the most and buys the least: collision detection,
scroll containers, resize, and RTL are exactly the hard parts, and writing them for 100 % of
visitors when 84 % of them have a browser that does it correctly during layout is work that
degrades what the majority gets.

**D. CSS Anchor Positioning, with one fallback we own (chosen).** The mechanism is A. Where
`CSS.supports('anchor-name: --a')` is false, a single module — one for all four components —
measures the anchor and writes the offsets into custom properties on a stylesheet rule, which is
what the CSP permits. It is C's code, but it is C's code on the path that is shrinking rather than
on the path everybody takes, and every browser that gains support drops off it without a release
of ours.

## Consequences

* One module, `src/positioning.ts`, owned by `F1-06`. `Tooltip`, `Menu`, `Popover` and the drawer
  side of `Dialog` use it; no component measures anything itself.
* The fallback is a *fallback*: the CSS path is what is styled and reviewed, and the workbench
  shows the CSS path. A story cannot show both, so the fallback needs a test of its own rather
  than an axis.
* No new dependency. If the fallback turns out to be more than roughly a hundred lines, that is
  the signal to reopen option B — with a number, the way `ADR-0037` reopened Storybook's.
* `position-area` and `position-try` are logical: they follow the writing direction, so RTL costs
  nothing here. The fallback has to do the same by hand, which is the first thing its test checks.

## What is still open

`F1-13`'s brief says the choice is "checked against `support-matrix.md`" — and `support-matrix.md`
has no browser row. It covers the server and `hubctl`; which browsers a Hubtask client is required
to work in has never been decided, and it is a support-scope decision rather than an implementation
one (`CLAUDE.md`, "what you do not decide yourself").

**D was chosen without that row, deliberately, because it is the only one of the four that is
correct whichever shape the row takes.** The numbers above are what the decision rests on, and two
plausible rows pull in opposite directions:

* **"the current and the previous major of each engine"** — then every supported browser has
  anchor positioning, the fallback in D is dead code, and A is simply right.
* **"anything still receiving security updates"** — then the fallback is load-bearing for years
  and B's argument gets stronger.

D is A with B's insurance, so neither row invalidates it. What the row *will* decide is how long
the fallback has to live — and therefore when it can be deleted, which is a smaller question than
which mechanism to build on. The row stays an open point in `design-system.md` §9 for that reason
rather than as a condition on this decision.
