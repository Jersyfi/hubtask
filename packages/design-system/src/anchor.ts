// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Where an overlay is drawn, decided once for the four components that need it (ADR-0039).
//
// The mechanism is CSS Anchor Positioning: `anchor-name` on the trigger, `position-area` on the
// overlay, `position-try-fallbacks` for the flip. It happens during layout rather than a frame
// later, it costs no dependency, and every engine on the support row has it - which the `engines`
// job asks each of them, every run (ADR-0048, support-matrix.md §5). The measured fallback this
// file once carried beside it went with that job: unreachable by any engine the client promises
// to run in, and proven so rather than assumed (F6-02).
//
// One constraint shapes the lines below. **No inline style.** ADR-0028's policy is
// `style-src 'self'` with no `'unsafe-inline'`, so `element.style.…` is exactly what may not be
// written - and neither may the `anchor-name` the CSS path needs, which is a per-instance value.
// Both go into rules in one constructed stylesheet, which is what ADR-0039 permits.

/** Which side of the anchor the overlay is drawn on. Logical: `inline-start` follows the text. */
export type Side = 'block-start' | 'block-end' | 'inline-start' | 'inline-end';

/** How it lines up along the other axis. */
export type Alignment = 'start' | 'center' | 'end';

export interface Placement {
  readonly side: Side;
  readonly align: Alignment;
}

const isBlockAxis = (side: Side) => side === 'block-start' || side === 'block-end';

/**
 * `position-area`, which is logical, so one table serves both directions. `span-inline-end` runs
 * from the anchor's inline start towards its end, which is what `align: 'start'` means here;
 * getting this table backwards is a bug that only shows up as "the menu is left-aligned in Chrome
 * and right-aligned in Firefox".
 */
export function positionArea({ side, align: alignment }: Placement): string {
  if (alignment === 'center') return side;
  const axis = isBlockAxis(side) ? 'inline' : 'block';
  const span = alignment === 'start' ? `span-${axis}-end` : `span-${axis}-start`;
  return `${side} ${span}`;
}

let sheet: CSSStyleSheet | undefined;
let sequence = 0;

function stylesheet(): CSSStyleSheet {
  if (!sheet) {
    sheet = new CSSStyleSheet();
    document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
  }
  return sheet;
}

/** The rule this overlay owns, created on first use and dropped when the overlay closes. */
function ruleFor(target: CSSStyleSheet, selector: string): CSSStyleRule {
  const index = target.insertRule(`${selector} {}`, target.cssRules.length);
  return target.cssRules[index] as CSSStyleRule;
}

const drop = (target: CSSStyleSheet, rule: CSSRule) => {
  const index = [...target.cssRules].indexOf(rule);
  if (index >= 0) target.deleteRule(index);
};

export interface AnchorOptions {
  readonly placement: Placement;
}

/**
 * Puts the overlay in the top layer, and returns the function that takes it out again.
 *
 * The top layer is what anchor positioning is designed to pair with, and it is the only answer to
 * a question a stacking scale cannot reach: an overlay is laid out inside any ancestor that is a
 * containing block for fixed elements - a transform, a filter, `contain` - and clipped by its
 * `overflow`. That ancestor is not always ours. A card that lifts on hover is a transform, and a
 * menu opened from inside it would be drawn in the card.
 *
 * `manual` rather than `auto`: light dismiss would close the overlay on its own and take the
 * `Escape` the register in `layers.ts` is supposed to answer.
 *
 * Where the browser has no `showPopover`, nothing is raised and the overlay stays where it was.
 * The attribute is removed again on the way out *and* if raising fails, because a `[popover]`
 * that was never shown is `display: none`, and an overlay nobody can see is worse than one drawn
 * in the wrong place.
 */
export function raiseToTopLayer(overlay: HTMLElement): () => void {
  if (typeof overlay.showPopover !== 'function') return () => {};

  overlay.setAttribute('popover', 'manual');
  try {
    overlay.showPopover();
  } catch {
    // The attribute goes straight back off, and this is the branch that matters most in the file:
    // a `[popover]` that was never shown is `display: none`, so leaving it on would not misplace
    // the overlay - it would delete it. Better drawn in the wrong box than not drawn at all.
    overlay.removeAttribute('popover');
    return () => {};
  }

  return () => {
    try {
      overlay.hidePopover();
    } catch {
      // Already hidden, or already gone from the document. Either way there is nothing to lower.
    }
    overlay.removeAttribute('popover');
  };
}

/**
 * Ties an overlay to its trigger and returns the function that unties it: two declarations and no
 * listeners, the flip done by the engine during layout rather than a frame after it.
 */
export function anchorTo(trigger: HTMLElement, overlay: HTMLElement, { placement }: AnchorOptions): () => void {
  const id = `hbt-anchor-${++sequence}`;
  trigger.dataset.hbtAnchor = id;
  overlay.dataset.hbtAnchored = id;

  const target = stylesheet();
  const anchorRule = ruleFor(target, `[data-hbt-anchor='${id}']`);
  const overlayRule = ruleFor(target, `[data-hbt-anchored='${id}']`);
  const lower = raiseToTopLayer(overlay);

  // The user agent gives a `[popover]` element `inset: 0` and centres it with `margin: auto`; the
  // rule below says where the overlay goes, so it has to say that it means it.
  overlayRule.style.setProperty('inset', 'auto');
  anchorRule.style.setProperty('anchor-name', `--${id}`);
  overlayRule.style.setProperty('position-anchor', `--${id}`);
  overlayRule.style.setProperty('position-area', positionArea(placement));
  // `flip-block` and `flip-inline` are the two the sides above can need.
  overlayRule.style.setProperty('position-try-fallbacks', 'flip-block, flip-inline');

  return () => {
    lower();
    drop(target, anchorRule);
    drop(target, overlayRule);
    delete trigger.dataset.hbtAnchor;
    delete overlay.dataset.hbtAnchored;
  };
}

/**
 * Lays a spotlight over an element: the cut-out takes the element's box, by the same mechanism
 * `anchorTo` uses and with no measuring - `anchor()` for its edges and `anchor-size()` for its
 * size, resolved by the engine during layout, so the cut-out follows the element through a
 * resize or a scroll without a listener (ADR-0039; the onboarding tour, F6-14). The air around
 * the element is the caller's stylesheet's, as a margin on the cut-out.
 *
 * Raised to the top layer like every overlay, and first: the coach mark that is anchored to the
 * cut-out has to come after it in the top layer for the anchor to be acceptable.
 */
export function spotlightTo(target: HTMLElement, cutout: HTMLElement): () => void {
  const id = `hbt-spot-${++sequence}`;
  target.dataset.hbtSpotlit = id;
  cutout.dataset.hbtSpotlight = id;

  const sheet = stylesheet();
  const targetRule = ruleFor(sheet, `[data-hbt-spotlit='${id}']`);
  const cutoutRule = ruleFor(sheet, `[data-hbt-spotlight='${id}']`);
  const lower = raiseToTopLayer(cutout);

  targetRule.style.setProperty('anchor-name', `--${id}`);
  cutoutRule.style.setProperty('position-anchor', `--${id}`);
  cutoutRule.style.setProperty('inset', 'auto');
  cutoutRule.style.setProperty('top', `anchor(--${id} top)`);
  cutoutRule.style.setProperty('left', `anchor(--${id} left)`);
  cutoutRule.style.setProperty('width', `anchor-size(--${id} width)`);
  cutoutRule.style.setProperty('height', `anchor-size(--${id} height)`);

  return () => {
    lower();
    drop(sheet, targetRule);
    drop(sheet, cutoutRule);
    delete target.dataset.hbtSpotlit;
    delete cutout.dataset.hbtSpotlight;
  };
}
