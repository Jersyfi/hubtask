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
// A surface with a tip - a reason bubble, a tooltip, a help tip - is the one exception: its tip
// must know the side it took, which CSS does not tell, so it is measured (`placeWithTip`).
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
  /** A surface with a tip pointing at its anchor: placed by `placeWithTip`, not by the engine. */
  readonly tip?: TipOptions;
}

export interface TipOptions {
  /** What the tip points at, where that is not the anchor: the ⓘ of a sheet's row. */
  readonly aim?: HTMLElement | null;
  /**
   * The surface the anchor stands in - a menu around its row. Beside it, the overlay stands clear
   * of that surface's edge rather than of the row's.
   */
  readonly within?: HTMLElement | null;
}

/** A physical side, as the viewport and a rotated tip see it. */
export type PhysicalSide = 'top' | 'bottom' | 'left' | 'right';

/**
 * The sides a surface with a tip tries, in order (design-system.md §4, "Where a bubble stands").
 * Beside the anchor (a reason in a menu): that side, the other, below, above. Above or below (a
 * field's reason, a tooltip, a help tip): that side, then the other.
 */
export function tipSides({ side }: Placement, dir: 'ltr' | 'rtl'): PhysicalSide[] {
  const start: PhysicalSide = dir === 'rtl' ? 'right' : 'left';
  const end: PhysicalSide = dir === 'rtl' ? 'left' : 'right';
  switch (side) {
    case 'inline-end': return [end, start, 'bottom', 'top'];
    case 'inline-start': return [start, end, 'bottom', 'top'];
    case 'block-end': return ['bottom', 'top'];
    default: return ['top', 'bottom'];
  }
}

/** A box in the viewport by its edges. */
export interface Edges {
  readonly top: number;
  readonly left: number;
  readonly right: number;
  readonly bottom: number;
}

export interface TipInput {
  /** What the surface stands beside. */
  readonly anchor: Edges;
  /** What the surface clears on the left and right sides, where that is wider: the anchor's menu. */
  readonly beside?: Edges;
  /** What the tip points at. */
  readonly aim: Edges;
  readonly width: number;
  readonly height: number;
  readonly viewport: { readonly width: number; readonly height: number };
  readonly sides: readonly PhysicalSide[];
  /** Above or below: where along the anchor's inline axis the surface starts. */
  readonly dir: 'ltr' | 'rtl';
  /** From the anchor to the surface's edge: the tip and the air before its point. */
  readonly gap: number;
  /** From the viewport's edge to the surface's, as far as the surface can. */
  readonly edge: number;
  /** The least of that, given up only so that the tip reaches its aim on a straight edge. */
  readonly nearEdge: number;
  /** The surface's corner radius. The tip's outer extent never enters it. */
  readonly radius: number;
  /** How far the tip stands out; a square turned by 45°, so as far along the edge each way. */
  readonly tip: number;
  /** Where the tip points into an aim wider than the surface: this far past its start. */
  readonly wideAim: number;
}

export interface TipPlacement {
  /** The side of the anchor the surface stands on. */
  readonly side: PhysicalSide;
  /** The surface's border box, in the viewport. */
  readonly top: number;
  readonly left: number;
  /** The tip's centre along the edge that faces the anchor, from that edge's top or left end. */
  readonly at: number;
}

const clamp = (value: number, low: number, high: number) => Math.max(low, Math.min(high, value));

/**
 * Where a surface with a tip stands: pure, so its rules are tested without a browser.
 *
 * The first side in `sides` that holds the surface at its own size; where none does, the one with
 * the most room. Across that side it starts where the anchor starts - above or below, at the
 * anchor's inline start; beside it, at its top, or at its bottom where the top runs past the foot -
 * and is shifted into the viewport, `edge` from its border. The tip then points at the aim and
 * never enters a rounded corner: its outer extent stays `radius` from each end of its edge. Where
 * that is not enough to reach the aim, the surface comes nearer the viewport's edge, down to
 * `nearEdge`; only past that does the tip stop at the end of the straight edge.
 */
export function placeWithTip(input: TipInput): TipPlacement {
  const { anchor, aim, width, height, viewport, gap, edge, nearEdge, radius, tip } = input;
  const beside = input.beside ?? anchor;
  const room: Record<PhysicalSide, number> = {
    top: anchor.top - edge - gap - height,
    bottom: viewport.height - edge - anchor.bottom - gap - height,
    left: beside.left - edge - gap - width,
    right: viewport.width - edge - beside.right - gap - width,
  };
  const side = input.sides.find((each) => room[each] >= 0)
    ?? [...input.sides].sort((one, other) => room[other] - room[one])[0]
    ?? 'top';

  const isBeside = side === 'left' || side === 'right';
  const size = isBeside ? height : width;
  const span = isBeside ? viewport.height : viewport.width;

  // Along the side: off the anchor by the gap, and back inside the viewport where nothing fit.
  let main: number;
  if (side === 'top') main = anchor.top - gap - height;
  else if (side === 'bottom') main = anchor.bottom + gap;
  else if (side === 'left') main = beside.left - gap - width;
  else main = beside.right + gap;
  const mainSize = isBeside ? width : height;
  const mainSpan = isBeside ? viewport.width : viewport.height;
  main = Math.max(edge, Math.min(main, mainSpan - edge - mainSize));

  // Across it: lined up with the anchor's start, then shifted into the viewport.
  let cross: number;
  if (isBeside) {
    cross = anchor.top + height > span - edge ? anchor.bottom - height : anchor.top;
  } else {
    cross = input.dir === 'rtl' ? anchor.right - width : anchor.left;
  }
  cross = Math.max(edge, Math.min(cross, span - edge - size));

  // The point the tip aims at, across the side.
  const aimSize = isBeside ? aim.bottom - aim.top : aim.right - aim.left;
  let target: number;
  if (isBeside) target = (aim.top + aim.bottom) / 2;
  else if (aimSize > width) target = input.dir === 'rtl' ? aim.right - input.wideAim : aim.left + input.wideAim;
  else target = (aim.left + aim.right) / 2;

  // The straight part of the edge the tip may take, and the surface moved for it where needed.
  const low = radius + tip;
  const high = size - radius - tip;
  if (high < low) return { side, ...corner(side, main, cross), at: size / 2 };
  if (target - cross < low) cross = Math.max(Math.min(nearEdge, cross), target - low);
  else if (target - cross > high) cross = Math.min(Math.max(span - nearEdge - size, cross), target - high);
  const at = clamp(target - cross, low, high);
  return { side, ...corner(side, main, cross), at };
}

const corner = (side: PhysicalSide, main: number, cross: number) =>
  side === 'left' || side === 'right' ? { top: cross, left: main } : { top: main, left: cross };

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
export function anchorTo(trigger: HTMLElement, overlay: HTMLElement, { placement, standalone = false, isInTopLayer = false, tip }: AnchorOptions): () => void {
  if (tip) return tipTo(trigger, overlay, placement, tip, isInTopLayer);
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

/** A length a custom property or a property resolves to, in pixels; 0 where it is none. */
const pixels = (style: CSSStyleDeclaration, name: string) => Number.parseFloat(style.getPropertyValue(name)) || 0;

const edgesOf = (element: Element): Edges => {
  const box = element.getBoundingClientRect();
  return { top: box.top, left: box.left, right: box.right, bottom: box.bottom };
};

/**
 * A surface with a tip, tied to its anchor. CSS anchoring cannot tell a surface which fallback it
 * took, and a tip has to know; nor can it let the surface nearer the viewport's edge only so far as
 * the tip needs. So this one is measured: `placeWithTip` decides, and the answer is written into the
 * surface's rule in the constructed stylesheet - its corner, and where its tip stands as
 * `--hbt-tip-inline` and `--hbt-tip-block` from its inline-start and block-start border edges -
 * with the side the tip points to as `data-hbt-tip`. Placed again on a scroll anywhere, a resize, and
 * a change of the anchor's or the surface's size, for as long as it is open.
 *
 * The surface declares its tip's reach as `--hbt-tip`, and sizes itself to its content (its width
 * never depends on where it stands, or the measurement would chase itself).
 */
function tipTo(anchor: HTMLElement, overlay: HTMLElement, placement: Placement, { aim, within }: TipOptions, isInTopLayer: boolean): () => void {
  const id = `hbt-anchor-${++sequence}`;
  overlay.dataset.hbtAnchored = id;
  const target = stylesheet();
  const rule = ruleFor(target, `[data-hbt-anchored='${id}']`);
  const lower = isInTopLayer ? () => {} : raiseToTopLayer(overlay);
  rule.style.setProperty('inset', 'auto');
  rule.style.setProperty('margin', '0', 'important');

  const place = () => {
    if (!overlay.isConnected || !anchor.isConnected) return;
    const style = getComputedStyle(overlay);
    const dir = getComputedStyle(anchor).direction === 'rtl' ? 'rtl' : 'ltr';
    const box = overlay.getBoundingClientRect();
    const reach = pixels(style, '--hbt-tip');
    const root = document.documentElement;
    const placed = placeWithTip({
      anchor: edgesOf(anchor),
      beside: within ? edgesOf(within) : undefined,
      aim: edgesOf(aim ?? anchor),
      width: box.width,
      height: box.height,
      viewport: { width: root.clientWidth, height: root.clientHeight },
      sides: tipSides(placement, dir),
      dir,
      gap: reach + pixels(style, '--sp-050'),
      edge: pixels(style, '--sp-200'),
      nearEdge: pixels(style, '--sp-100'),
      radius: Number.parseFloat(style.borderTopLeftRadius) || 0,
      tip: reach,
      wideAim: pixels(style, '--sp-300'),
    });
    // The tip's centre on the edge that faces the anchor, from the surface's top left corner.
    const isBeside = placed.side === 'left' || placed.side === 'right';
    const x = isBeside ? (placed.side === 'right' ? 0 : box.width) : placed.at;
    const y = isBeside ? placed.at : placed.side === 'bottom' ? 0 : box.height;
    const isRtl = style.direction === 'rtl';
    rule.style.setProperty('top', `${placed.top}px`);
    rule.style.setProperty('left', `${placed.left}px`);
    rule.style.setProperty('--hbt-tip-inline', `${isRtl ? box.width - x : x}px`);
    rule.style.setProperty('--hbt-tip-block', `${y}px`);
    overlay.dataset.hbtTip = { top: 'bottom', bottom: 'top', left: 'right', right: 'left' }[placed.side];
  };

  place();
  const options = { capture: true, passive: true } as const;
  document.addEventListener('scroll', place, options);
  window.addEventListener('resize', place);
  const observer = typeof ResizeObserver === 'function' ? new ResizeObserver(place) : undefined;
  observer?.observe(overlay);
  observer?.observe(anchor);

  return () => {
    document.removeEventListener('scroll', place, options);
    window.removeEventListener('resize', place);
    observer?.disconnect();
    lower();
    drop(target, rule);
    delete overlay.dataset.hbtAnchored;
    delete overlay.dataset.hbtTip;
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
