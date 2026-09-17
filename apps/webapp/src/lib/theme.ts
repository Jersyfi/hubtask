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
 * The visible System / Light / Dark switch (F5-12, `ProfileView`) keeps its choice on the device,
 * in the browser's storage until the local persistence port ADR-0033 defers arrives with the
 * shells: `readThemeChoice` and `storeThemeChoice` are that seam, and `startTheme` is what
 * `main.ts` and the switch both call. `followSystemTheme` returns its stop function for exactly
 * that caller.
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

/** What a device may choose: the system's word, or one of the two modes over it. */
export type ThemeChoice = 'system' | ThemeMode;

/** The key the choice is kept under on this device. */
export const THEME_KEY = 'hubtask.theme';

/** A place a choice is kept: `localStorage`'s two methods, or a fake in a test. */
export interface ChoiceStore {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

/** The choice kept on this device, or `system` where none is or the store cannot be reached. */
export function readThemeChoice(store: ChoiceStore | undefined): ThemeChoice {
  try {
    const kept = store?.getItem(THEME_KEY);
    return kept === 'light' || kept === 'dark' ? kept : 'system';
  } catch {
    return 'system';
  }
}

/** Keeps the choice on this device; `system` removes it, because following the system is the absence of a choice. */
export function storeThemeChoice(store: ChoiceStore | undefined, choice: ThemeChoice): void {
  try {
    if (choice === 'system') store?.removeItem(THEME_KEY);
    else store?.setItem(THEME_KEY, choice);
  } catch {
    // A store that refuses (private mode, a quota) loses the choice for the next visit and nothing else.
  }
}

/**
 * Applies a choice: the system's preference followed live, or one mode set and left. Returns the
 * stop function either way, so a switch can replace one choice with the next.
 */
export function startTheme(choice: ThemeChoice): () => void {
  if (choice === 'system') return followSystemTheme();
  applyTheme(document.documentElement, choice === 'dark');
  return () => {};
}
