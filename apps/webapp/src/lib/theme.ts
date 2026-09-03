// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The theme, set deliberately on the document (W-07). The generated stylesheet has no `:root`
 * fallback on purpose — a document without `data-theme` looks broken at once rather than
 * quietly picking a mode nobody chose (ADR-0029, the design-system README). So this module is
 * the one place that sets it, and it follows the system preference.
 *
 * **There is no account preference for it, and that is a decision rather than a gap**
 * ([ADR-0043](../../../../docs/adr/ADR-0043-theme-per-device.md)). Language, time zone and week
 * start are properties of the person and resolve through the account (`i18n-l10n.md` §2); the
 * theme is the one that is legitimately different per device — dark on a phone at night, light on
 * a bright monitor — which is why the operating system exposes it per device and why following it
 * is the right default rather than a placeholder.
 *
 * A visible System / Light / Dark switch therefore keeps its choice on the device, which needs the
 * local persistence port ADR-0033 defers to the shell and sync work. `followSystemTheme` returns
 * its stop function for exactly that caller.
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
 * Follow `prefers-color-scheme`, now and on every change. Returns the stop function: a device-level
 * override calls it and sets the attribute itself (ADR-0043), which is the only thing that ever
 * outranks the system here.
 */
export function followSystemTheme(): () => void {
  const query = window.matchMedia('(prefers-color-scheme: dark)');
  const apply = () => applyTheme(document.documentElement, query.matches);
  apply();
  query.addEventListener('change', apply);
  return () => query.removeEventListener('change', apply);
}
