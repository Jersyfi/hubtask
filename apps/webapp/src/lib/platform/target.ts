// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which implementation of the seam a build carries (ADR-0033 §1): chosen by the build, never by
 * sniffing at run time. `vite.config.ts` asks `chooseTarget` with the build's mode and redirects
 * `index.ts`'s one import of the browser implementation to the target's own, so a component - and
 * the browser bundle - never sees another target's module.
 */

/** The targets a build can choose. `mobile` arrives with the Android shell. */
export type BuildTarget = 'browser' | 'desktop';

/** The mode that builds the desktop shell's bundle: `vite build --mode desktop`. */
export const DESKTOP_MODE = 'desktop';

/**
 * The target a Vite mode builds. Every mode that names no shell - `production`, `development`,
 * `test` - is the browser's: the browser is the primary target, and a mode added for another reason
 * must not turn a build into a shell's.
 */
export function chooseTarget(mode: string | undefined): BuildTarget {
  return mode === DESKTOP_MODE ? 'desktop' : 'browser';
}

/** The implementation each target carries, beside `index.ts`. */
export const IMPLEMENTATION: Readonly<Record<BuildTarget, string>> = {
  browser: './browser.ts',
  desktop: './desktop.ts',
};

/**
 * Where an import resolves for `target`: `index.ts`'s import of the browser implementation goes to
 * the target's, and every other import stays where it was. `undefined` means "unchanged".
 */
export function redirect(source: string, importer: string | undefined, target: BuildTarget): string | undefined {
  if (target === 'browser' || source !== IMPLEMENTATION.browser) return undefined;
  if (importer === undefined || !/[\\/]lib[\\/]platform[\\/]index\.ts$/.test(importer)) return undefined;
  return IMPLEMENTATION[target];
}
