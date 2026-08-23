// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The theme, set deliberately on the document (W-07). The generated stylesheet has no `:root`
 * fallback on purpose — a document without `data-theme` looks broken at once rather than
 * quietly picking a mode nobody chose (ADR-0029, the design-system README). So this module is
 * the one place that sets it, and it follows the system preference until the account
 * preference exists to override it.
 */

/** The two modes `tokens.css` defines. */
export type ThemeMode = 'light' | 'dark';

/** The seam node/test code can reach: everything of the document this module touches. */
export interface ThemeTarget {
  setAttribute(name: string, value: string): void;
}

export function applyTheme(root: ThemeTarget, prefersDark: boolean): ThemeMode {
  const mode: ThemeMode = prefersDark ? 'dark' : 'light';
  root.setAttribute('data-theme', mode);
  return mode;
}

/**
 * Follow `prefers-color-scheme`, now and on every change. Returns the stop function; the
 * account preference, when it exists, will call it and set the attribute itself.
 */
export function followSystemTheme(): () => void {
  const query = window.matchMedia('(prefers-color-scheme: dark)');
  const apply = () => applyTheme(document.documentElement, query.matches);
  apply();
  query.addEventListener('change', apply);
  return () => query.removeEventListener('change', apply);
}
