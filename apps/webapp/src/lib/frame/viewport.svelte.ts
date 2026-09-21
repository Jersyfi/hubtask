// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which of the five widths the frame is drawn at (design-system.md §6), read once for the whole
 * application rather than by every component that draws differently on a phone.
 *
 * Width is not platform (ADR-0061): whether a bar or a drawer appears is answered here by a media
 * query and never by `src/lib/platform/`, so the desktop shell dragged to 500 px behaves like a
 * phone. The thresholds are the tokens' — `--bp-medium` and `--bp-expanded` read from the
 * stylesheet at start, the way `PageHeader` reads its own — because a media query cannot read a
 * custom property and a number written here would be a value outside `tokens.json`.
 *
 * The pointer is the third question, beside the two widths: `density.spacious` is for a thumb
 * as much as for a small screen, and a tablet with a finger on it is not a phone by width.
 */

export class Viewport {
  #isCompact = $state(false);
  #isBelowExpanded = $state(false);
  #isCoarse = $state(false);

  /** Below `medium`: one column, the bottom bar, `density.spacious`. */
  get isCompact(): boolean {
    return this.#isCompact;
  }

  /** Below `expanded`: the navigation is a drawer rather than pinned. */
  get isBelowExpanded(): boolean {
    return this.#isBelowExpanded;
  }

  /** The primary pointer is a finger rather than a mouse. */
  get isCoarse(): boolean {
    return this.#isCoarse;
  }

  /** Where the frame is drawn for a thumb: below `medium`, or wherever the pointer is coarse. */
  get isSpacious(): boolean {
    return this.#isCompact || this.#isCoarse;
  }

  /** Starts listening. Nothing happens where there is no window - a test - and the widths stay wide. */
  start(): () => void {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return () => {};
    const style = getComputedStyle(document.documentElement);
    const medium = Number.parseFloat(style.getPropertyValue('--bp-medium'));
    const expanded = Number.parseFloat(style.getPropertyValue('--bp-expanded'));

    const stops: (() => void)[] = [];
    const watch = (query: string, apply: (matches: boolean) => void) => {
      const list = window.matchMedia(query);
      const onChange = () => apply(list.matches);
      onChange();
      list.addEventListener('change', onChange);
      stops.push(() => list.removeEventListener('change', onChange));
    };

    // The range syntax is on the browser row (ADR-0044); "less than" says "below medium" without
    // a minus one that would be a second number to keep in step with the token.
    if (Number.isFinite(medium)) watch(`(width < ${medium}px)`, (matches) => (this.#isCompact = matches));
    if (Number.isFinite(expanded)) watch(`(width < ${expanded}px)`, (matches) => (this.#isBelowExpanded = matches));
    watch('(pointer: coarse)', (matches) => (this.#isCoarse = matches));

    return () => {
      for (const stop of stops) stop();
    };
  }
}

export const viewport = new Viewport();
