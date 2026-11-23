// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Where an overlay is drawn, decided once for the four components that need it (ADR-0039).
//
// The mechanism is CSS Anchor Positioning: `anchor-name` on the trigger, `position-area` on the
// overlay, `position-try-fallbacks` for the flip. It happens during layout rather than a frame
// later, it costs no dependency, and every engine on the support row has it - which the `engines`
// job asks each of them, every run (ADR-0048, support-matrix.md §5). The measured fallback this
// file once carried beside it went with that job: unreachable by any engine the client promises
// to run in, and proven so rather than assumed.
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

/**
 * The last resort, for a list taller than the room on either side of its trigger - a long menu
 * opened from a row near the foot of a phone's screen. The overlay takes the side with more room,
 * shrinks to it and scrolls inside it, and never runs past the viewport.
 *
 * Each line answers a way the plain flips fail:
 *
 * - `span-all` and `anchor-center`: a surface as wide as its longest item can be wider than the
 *   room on either side of a trigger in the middle of a row. Every option that keeps one side then
 *   overflows sideways, the engine finds none that fits and keeps the first - below, past the
 *   foot - whatever the block size does. Across the whole width, centred on the trigger and
 *   pushed back inside the viewport, the surface always fits sideways.
 * - `stretch`: what resolves against the room beside the trigger. A percentage resolves against
 *   the viewport, so `max-height: 100%` changes nothing.
 * - The minimum chooses the side. It is half of what the trigger leaves of the viewport, less some
 *   slack for the overlay's margin, so the smaller side cannot hold it and is refused, and the
 *   other option takes the larger one. `position-try-order: most-block-size` would choose too,
 *   but it sorts the overlay's own position with the rest and would open every short menu upwards
 *   whenever there is more room above.
 * - `--hbt-fit-slack`: the minimum is a content box, and an overlay's padding and border come on
 *   top of it. Where they are wider than the slack, both sides of a trigger near the middle are
 *   refused and the engine keeps the first position - below, pushed back up over the trigger. An
 *   overlay with a wide frame (`Popover`) declares a slack that holds it.
 */
const FIT = '--hbt-fit';
const FIT_RULE = `@position-try ${FIT} {
  position-area: block-end span-all;
  justify-self: anchor-center;
  max-block-size: -webkit-fill-available;
  max-block-size: stretch;
  min-block-size: calc((100dvh - anchor-size(block)) / 2 - var(--hbt-fit-slack, var(--sp-200)));
}`;

/**
 * Five at most: Chromium tries no more than five fallbacks and ignores the rest, so a sixth that
 * would fit is never reached and the overlay stays where it was put first. The shrunk option
 * covers both sides and both inline directions, which is why two of it are enough.
 */
const FALLBACKS = `flip-block, flip-inline, flip-block flip-inline, ${FIT}, ${FIT} flip-block`;

/**
 * A surface that stands alone beside its anchor - a tour's mark - and must never shrink: its words
 * and its buttons are the whole of it. Where no side of the anchor has room (a target as tall as
 * the screen), it stands in the viewport's end corner, over the page, rather than clipped.
 */
const OVER = '--hbt-over';
const OVER_RULE = `@position-try ${OVER} {
  position-area: span-all;
  align-self: end;
  justify-self: end;
}`;
const STANDALONE = `flip-block, flip-inline, flip-block flip-inline, ${OVER}`;

let sheet: CSSStyleSheet | undefined;
let sequence = 0;

function stylesheet(): CSSStyleSheet {
  if (!sheet) {
    sheet = new CSSStyleSheet();
    sheet.insertRule(FIT_RULE);
    sheet.insertRule(OVER_RULE);
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
  /** A surface that must keep its size: never shrunk, put over the page where no side fits. */
  readonly standalone?: boolean;
  /** An overlay already in the top layer - a modal dialog - is tied, not raised a second time. */
  readonly isInTopLayer?: boolean;
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
export function anchorTo(trigger: HTMLElement, overlay: HTMLElement, { placement, standalone = false, isInTopLayer = false }: AnchorOptions): () => void {
  const id = `hbt-anchor-${++sequence}`;
  trigger.dataset.hbtAnchor = id;
  overlay.dataset.hbtAnchored = id;

  const target = stylesheet();
  const anchorRule = ruleFor(target, `[data-hbt-anchor='${id}']`);
  const overlayRule = ruleFor(target, `[data-hbt-anchored='${id}']`);
  const lower = isInTopLayer ? () => {} : raiseToTopLayer(overlay);

  // The user agent gives a `[popover]` element `inset: 0` and centres it with `margin: auto`; the
  // rule below says where the overlay goes, so it has to say that it means it.
  overlayRule.style.setProperty('inset', 'auto');
  anchorRule.style.setProperty('anchor-name', `--${id}`);
  overlayRule.style.setProperty('position-anchor', `--${id}`);
  overlayRule.style.setProperty('position-area', positionArea(placement));
  // `flip-block` and `flip-inline` are the two the sides above can need; the shrunk one comes last.
  overlayRule.style.setProperty('position-try-fallbacks', standalone ? STANDALONE : FALLBACKS);

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
 * resize or a scroll without a listener (ADR-0039; the onboarding tour). The air around
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

/** A box in the viewport, as `getBoundingClientRect` answers it. */
export interface Box {
  readonly top: number;
  readonly left: number;
  readonly width: number;
  readonly height: number;
}

/** Where a container's content box starts in the viewport, and how far it is scrolled. */
export interface Frame extends Box {
  readonly borderTop: number;
  readonly borderLeft: number;
  readonly scrollTop: number;
  readonly scrollLeft: number;
  readonly clientWidth: number;
}

/** A box inside a container, in logical terms: what the phantom's rule is written in. */
export interface PlacedBox {
  readonly blockStart: number;
  readonly inlineStart: number;
  readonly inlineSize: number;
  readonly blockSize: number;
}

/**
 * A viewport box read inside a container's padding box, logically. In a right-to-left container
 * the inline start is the right edge, measured from the container's own right edge.
 */
export function boxWithin(box: Box, frame: Frame, dir: 'ltr' | 'rtl' = 'ltr'): PlacedBox {
  const blockStart = box.top - frame.top - frame.borderTop + frame.scrollTop;
  const fromLeft = box.left - frame.left - frame.borderLeft + frame.scrollLeft;
  const inlineStart = dir === 'rtl' ? frame.clientWidth - (fromLeft + box.width) : fromLeft;
  return { blockStart, inlineStart, inlineSize: box.width, blockSize: box.height };
}

export interface RectAnchor {
  /** The element an overlay is anchored to with `anchorTo`. */
  readonly element: HTMLElement;
  /** Moves the anchor over a viewport box, or takes it away (`undefined`). */
  place(box: Box | undefined): void;
  release(): void;
}

/** How many box anchors each container holds. */
let hosting: WeakMap<HTMLElement, number> | undefined;

/**
 * An anchor for what is no element: a text selection or one line of an editor. Anchor positioning
 * needs an element, so an empty, hidden one is laid over the box inside a container, and the
 * overlay is tied to it as to any trigger. The container scrolls the phantom with the content, so
 * the overlay follows the text without a listener; only a new box (the selection changed) is
 * measured again, by the caller, and handed to `place`.
 *
 * The box is a rule in the constructed stylesheet, like every per-instance value here: no inline
 * style. The container is made the phantom's containing block by a rule as well; it must not be
 * an element whose children another library owns (an editor's own root). The phantom is empty
 * rather than hidden: an overlay whose anchor has `visibility: hidden` is not drawn
 * (`position-visibility: anchors-visible`, the engines' default).
 */
export function rectAnchor(container: HTMLElement): RectAnchor {
  const id = `hbt-rect-${++sequence}`;
  const target = stylesheet();
  const boxRule = ruleFor(target, `[data-hbt-rect='${id}']`);
  // Several anchors share a container (the bar, the handle, the panel): it is counted, not named,
  // or the first to go would take the containing block from the others.
  if (!hosting) {
    hosting = new WeakMap();
    target.insertRule(`[data-hbt-rect-host] { position: relative; }`, target.cssRules.length);
  }
  const held = (hosting.get(container) ?? 0) + 1;
  hosting.set(container, held);
  boxRule.style.setProperty('position', 'absolute');
  boxRule.style.setProperty('pointer-events', 'none');
  boxRule.style.setProperty('display', 'none');

  const phantom = document.createElement('span');
  phantom.setAttribute('aria-hidden', 'true');
  phantom.dataset.hbtRect = id;
  container.dataset.hbtRectHost = '';
  container.append(phantom);

  return {
    element: phantom,
    place(box) {
      if (!box) {
        boxRule.style.setProperty('display', 'none');
        return;
      }
      const outer = container.getBoundingClientRect();
      const placed = boxWithin(
        box,
        {
          top: outer.top, left: outer.left, width: outer.width, height: outer.height,
          borderTop: container.clientTop, borderLeft: container.clientLeft,
          scrollTop: container.scrollTop, scrollLeft: container.scrollLeft, clientWidth: container.clientWidth,
        },
        getComputedStyle(container).direction === 'rtl' ? 'rtl' : 'ltr',
      );
      boxRule.style.setProperty('display', 'block');
      boxRule.style.setProperty('inset-block-start', `${placed.blockStart}px`);
      boxRule.style.setProperty('inset-inline-start', `${placed.inlineStart}px`);
      boxRule.style.setProperty('inline-size', `${placed.inlineSize}px`);
      boxRule.style.setProperty('block-size', `${placed.blockSize}px`);
    },
    release() {
      phantom.remove();
      drop(target, boxRule);
      const left = (hosting?.get(container) ?? 1) - 1;
      hosting?.set(container, left);
      if (left === 0) delete container.dataset.hbtRectHost;
    },
  };
}
