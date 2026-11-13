// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Whether the window is a phone's, asked at the moment something opens - a status's steps as a
// sheet rather than a popover (work-surface concept §7). Layout itself stays in media queries; this
// answers only the question a stylesheet cannot: which surface to open.

/** Below `primitive.breakpoint.medium`: the compact width, where a menu becomes a sheet. */
// design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query string cannot read a custom property.
const COMPACT = '(width < 600px)';

export function isCompactWidth(): boolean {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function' && window.matchMedia(COMPACT).matches;
}

/** Calls back whenever the window crosses into or out of the compact width; returns the stop. */
export function watchCompactWidth(onchange: (isCompact: boolean) => void): () => void {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return () => {};
  const query = window.matchMedia(COMPACT);
  const listener = () => onchange(query.matches);
  query.addEventListener('change', listener);
  return () => query.removeEventListener('change', listener);
}
