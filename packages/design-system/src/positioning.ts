// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Where an overlay is drawn, decided once for the four components that need it (ADR-0039).
//
// The mechanism is CSS Anchor Positioning: `anchor-name` on the trigger, `position-area` on the
// overlay, `position-try-fallbacks` for the flip. Four engines ship it, it happens during layout
// rather than a frame later, and it costs no dependency. Where `CSS.supports` says the browser
// does not know it, one fallback - this file, for all four components - measures the anchor and
// writes the offsets instead.
//
// Two constraints shape every line below.
//
// **No inline style.** ADR-0028's policy is `style-src 'self'` with no `'unsafe-inline'`, so the
// obvious `element.style.insetInlineStart = …` is exactly what may not be written - and neither
// may the `anchor-name` the CSS path needs, which is a per-instance value. Both go into rules in
// one constructed stylesheet, which is what ADR-0039 permits.
//
// **The geometry is separable from the DOM.** `resolve` takes numbers and returns numbers, so the
// flip, the shift and - the first thing ADR-0039 asks of the fallback - the direction handling can
// be tested in plain Node. Everything that touches an element is below it and is a few lines.

/** Which side of the anchor the overlay is drawn on. Logical: `inline-start` follows the text. */
export type Side = 'block-start' | 'block-end' | 'inline-start' | 'inline-end';

/** How it lines up along the other axis. */
export type Alignment = 'start' | 'center' | 'end';

export interface Placement {
  readonly side: Side;
  readonly align: Alignment;
}

/** A box in the viewport's *logical* coordinates: inline follows the writing direction. */
export interface LogicalRect {
  readonly inlineStart: number;
  readonly blockStart: number;
  readonly inlineSize: number;
  readonly blockSize: number;
}

export interface Resolved {
  readonly inlineStart: number;
  readonly blockStart: number;
  /** The side actually used, which is not the side asked for when the overlay had to flip. */
  readonly side: Side;
}

const OPPOSITE: Record<Side, Side> = {
  'block-start': 'block-end',
  'block-end': 'block-start',
  'inline-start': 'inline-end',
  'inline-end': 'inline-start',
};

const isBlockAxis = (side: Side) => side === 'block-start' || side === 'block-end';

/** How much room `side` leaves between the anchor and the edge of the viewport. */
function room(side: Side, anchor: LogicalRect, viewport: LogicalRect): number {
  switch (side) {
    case 'block-start':
      return anchor.blockStart;
    case 'block-end':
      return viewport.blockSize - (anchor.blockStart + anchor.blockSize);
    case 'inline-start':
      return anchor.inlineStart;
    case 'inline-end':
      return viewport.inlineSize - (anchor.inlineStart + anchor.inlineSize);
  }
}

/** Where the overlay starts on the axis it is placed along. */
function place(side: Side, anchor: LogicalRect, size: number, gap: number): number {
  switch (side) {
    case 'block-start':
      return anchor.blockStart - size - gap;
    case 'block-end':
      return anchor.blockStart + anchor.blockSize + gap;
    case 'inline-start':
      return anchor.inlineStart - size - gap;
    case 'inline-end':
      return anchor.inlineStart + anchor.inlineSize + gap;
  }
}

/** …and where it starts on the other one, before the shift. */
function align(alignment: Alignment, start: number, anchorSize: number, size: number): number {
  if (alignment === 'center') return start + (anchorSize - size) / 2;
  return alignment === 'start' ? start : start + anchorSize - size;
}

/** Slides a run of `size` back inside `[0, extent]`, preferring the start when it fits neither. */
function shift(start: number, size: number, extent: number, margin: number): number {
  const last = extent - margin - size;
  if (last < margin) return margin;
  return Math.min(Math.max(start, margin), last);
}

/**
 * The whole positioning rule: flip to the opposite side when the asked-for one has no room and
 * the opposite does, then shift along the other axis so that nothing leaves the viewport.
 *
 * Everything here is logical, which is what makes RTL cost nothing: the caller converts once, in
 * `logicalRect`, and this function never learns which direction the document runs in.
 */
export function resolve(
  anchor: LogicalRect,
  overlay: { inlineSize: number; blockSize: number },
  viewport: LogicalRect,
  placement: Placement,
  gap = 0,
  margin = 0,
): Resolved {
  const wanted = placement.side;
  const alongBlock = isBlockAxis(wanted);
  const needed = (alongBlock ? overlay.blockSize : overlay.inlineSize) + gap + margin;

  // Flip only when it helps. An overlay that fits on neither side stays where it was asked to go
  // and is shifted into view, because a flip that swaps one overflow for another only moves the
  // problem to the side the reader was not looking at.
  const other = OPPOSITE[wanted];
  const side =
    room(wanted, anchor, viewport) >= needed || room(other, anchor, viewport) < needed ? wanted : other;

  const along = place(side, anchor, alongBlock ? overlay.blockSize : overlay.inlineSize, gap);
  const across = align(
    placement.align,
    alongBlock ? anchor.inlineStart : anchor.blockStart,
    alongBlock ? anchor.inlineSize : anchor.blockSize,
    alongBlock ? overlay.inlineSize : overlay.blockSize,
  );

  return alongBlock
    ? {
        side,
        blockStart: shift(along, overlay.blockSize, viewport.blockSize, margin),
        inlineStart: shift(across, overlay.inlineSize, viewport.inlineSize, margin),
      }
    : {
        side,
        inlineStart: shift(along, overlay.inlineSize, viewport.inlineSize, margin),
        blockStart: shift(across, overlay.blockSize, viewport.blockSize, margin),
      };
}

/**
 * A physical rect from the DOM, read as a logical one. In a horizontal writing mode the block axis
 * is the same either way and the inline axis is measured from the other edge in RTL - which is the
 * one line that makes the fallback behave like `position-area`, and the first thing its test
 * checks (ADR-0039).
 */
export function logicalRect(rect: DOMRectReadOnly, viewportInlineSize: number, dir: 'ltr' | 'rtl'): LogicalRect {
  return {
    inlineStart: dir === 'rtl' ? viewportInlineSize - rect.right : rect.left,
    blockStart: rect.top,
    inlineSize: rect.width,
    blockSize: rect.height,
  };
}

/** `position-area`, which is logical too, so one table serves both directions. */
export function positionArea({ side, align: alignment }: Placement): string {
  if (isBlockAxis(side)) {
    if (alignment === 'center') return side;
    return `${side} span-inline-${alignment === 'start' ? 'end' : 'start'}`;
  }
  if (alignment === 'center') return side;
  return `${side} span-block-${alignment === 'start' ? 'end' : 'start'}`;
}

// ---------------------------------------------------------------------------------------------
// The DOM half. One stylesheet, one rule per open overlay, nothing written to an element.
// ---------------------------------------------------------------------------------------------

/** Whether the browser knows anchor positioning. Read once: it cannot change within a document. */
let supported: boolean | undefined;

export function supportsAnchor(): boolean {
  if (supported === undefined) {
    supported =
      typeof CSS !== 'undefined' && typeof CSS.supports === 'function' && CSS.supports('anchor-name: --hbt');
  }
  return supported;
}

/** For the test, and for a workbench story that wants to show the path the majority does not take. */
export function overrideSupport(value: boolean | undefined): void {
  supported = value;
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

/**
 * The gap between the anchor and the overlay, read from the token rather than typed here: rule 15
 * says a length lives in tokens.json, and the fallback needs it as a number.
 */
function gapFrom(element: Element, property: string): number {
  const value = getComputedStyle(element).getPropertyValue(property).trim();
  return Number.parseFloat(value) || 0;
}

export interface AnchorOptions {
  readonly placement: Placement;
  /** The token whose length separates the overlay from its anchor. */
  readonly gapToken?: string;
  /** …and the one that keeps it off the edge of the viewport. */
  readonly marginToken?: string;
}

/**
 * Ties an overlay to its trigger and returns the function that unties it. On a browser with anchor
 * positioning that is two declarations and no listeners; on one without, it is the same two
 * elements measured on open, on scroll and on resize.
 */
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
 * Where the browser has no `showPopover` - older than anchor positioning, so always the fallback
 * path - nothing is raised and the overlay stays where it was. The attribute is removed again on
 * the way out *and* if raising fails, because a `[popover]` that was never shown is
 * `display: none`, and an overlay nobody can see is worse than one drawn in the wrong place.
 */
function raise(overlay: HTMLElement): () => void {
  if (typeof overlay.showPopover !== 'function') return () => {};

  overlay.setAttribute('popover', 'manual');
  try {
    overlay.showPopover();
  } catch {
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

export function anchorTo(
  trigger: HTMLElement,
  overlay: HTMLElement,
  { placement, gapToken = '--sp-050', marginToken = '--sp-100' }: AnchorOptions,
): () => void {
  const id = `hbt-anchor-${++sequence}`;
  trigger.dataset.hbtAnchor = id;
  overlay.dataset.hbtAnchored = id;

  const target = stylesheet();
  const anchorRule = ruleFor(target, `[data-hbt-anchor='${id}']`);
  const overlayRule = ruleFor(target, `[data-hbt-anchored='${id}']`);
  const lower = raise(overlay);

  // The user agent gives a `[popover]` element `inset: 0` and centres it with `margin: auto`. Both
  // paths below say where the overlay goes, so both have to say that they mean it.
  overlayRule.style.setProperty('inset', 'auto');

  if (supportsAnchor()) {
    anchorRule.style.setProperty('anchor-name', `--${id}`);
    overlayRule.style.setProperty('position-anchor', `--${id}`);
    overlayRule.style.setProperty('position-area', positionArea(placement));
    // The flip, done by the engine during layout rather than a frame after it. `flip-block` and
    // `flip-inline` are the two the sides above can need.
    overlayRule.style.setProperty('position-try-fallbacks', 'flip-block, flip-inline');
    return () => {
      lower();
      drop(target, anchorRule);
      drop(target, overlayRule);
      delete trigger.dataset.hbtAnchor;
      delete overlay.dataset.hbtAnchored;
    };
  }

  const dir = getComputedStyle(overlay).direction === 'rtl' ? 'rtl' : 'ltr';
  const gap = gapFrom(overlay, gapToken);
  const margin = gapFrom(overlay, marginToken);

  const update = () => {
    const viewportInline = document.documentElement.clientWidth;
    const viewport: LogicalRect = {
      inlineStart: 0,
      blockStart: 0,
      inlineSize: viewportInline,
      blockSize: document.documentElement.clientHeight,
    };
    const box = overlay.getBoundingClientRect();
    const position = resolve(
      logicalRect(trigger.getBoundingClientRect(), viewportInline, dir),
      { inlineSize: box.width, blockSize: box.height },
      viewport,
      placement,
      gap,
      margin,
    );
    overlayRule.style.setProperty('position', 'fixed');
    overlayRule.style.setProperty('inset-inline-start', `${position.inlineStart}px`);
    overlayRule.style.setProperty('inset-block-start', `${position.blockStart}px`);
  };

  update();
  // `true` on both: a scroll inside a container does not bubble, and an overlay anchored to
  // something in a scrolling panel is the case that looks broken without it.
  window.addEventListener('scroll', update, true);
  window.addEventListener('resize', update);

  return () => {
    window.removeEventListener('scroll', update, true);
    window.removeEventListener('resize', update);
    lower();
    drop(target, anchorRule);
    drop(target, overlayRule);
    delete trigger.dataset.hbtAnchor;
    delete overlay.dataset.hbtAnchored;
  };
}
