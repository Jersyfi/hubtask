// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Cutting and counting text the way a reader sees it (i18n-l10n.md §6 line 8, §5; F5-09).
 *
 * A JavaScript string is UTF-16 units, the server counts code points, and a reader sees
 * graphemes: a flag is two code points and four units, a family emoji is seven code points, and
 * `slice(0, 80)` in the middle of any of them hands a reader half a character. So a cut is made
 * between graphemes with `Intl.Segmenter` where the engine has it (every engine on the support
 * row does), and a length is counted the way the server counts it.
 */

const segmenters = new Map<string, Intl.Segmenter>();

function segmenterFor(locale: string): Intl.Segmenter | undefined {
  if (typeof Intl.Segmenter !== 'function') return undefined;
  let segmenter = segmenters.get(locale);
  if (!segmenter) {
    segmenter = new Intl.Segmenter(locale, { granularity: 'grapheme' });
    segmenters.set(locale, segmenter);
  }
  return segmenter;
}

/** The text as the graphemes a reader sees. Code points where the engine cannot segment. */
export function graphemes(text: string, locale = 'en'): string[] {
  const segmenter = segmenterFor(locale);
  if (!segmenter) return Array.from(text);
  return Array.from(segmenter.segment(text), (segment) => segment.segment);
}

/** The first `max` graphemes, never a partial one. */
export function truncateGraphemes(text: string, max: number, locale = 'en'): string {
  const parts = graphemes(text, locale);
  return parts.length <= max ? text : parts.slice(0, max).join('');
}

/** How long a text is as the server counts it: code points, not UTF-16 units (§5). */
export function codePoints(text: string): number {
  return Array.from(text).length;
}
