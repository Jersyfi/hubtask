// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which implementation of the seam a build carries (ADR-0033 §1): chosen by the build, never by
 * sniffing at run time. `vite.config.ts` asks `chooseTarget` with the build's mode and redirects
 * `index.ts`'s one import of the browser implementation to the target's own, so a component - and
 * the browser bundle - never sees another target's module.
 */

/** The targets a build can choose. */
export type BuildTarget = 'browser' | 'desktop' | 'mobile';

/** The mode that builds the desktop shell's bundle: `vite build --mode desktop`. */
export const DESKTOP_MODE = 'desktop';
/** The mode that builds the Android app's bundle: `vite build --mode mobile`. */
export const MOBILE_MODE = 'mobile';

/**
 * The target a Vite mode builds. Every mode that names no shell - `production`, `development`,
 * `test` - is the browser's: the browser is the primary target, and a mode added for another reason
 * must not turn a build into a shell's.
 */
export function chooseTarget(mode: string | undefined): BuildTarget {
  return mode === DESKTOP_MODE ? 'desktop' : mode === MOBILE_MODE ? 'mobile' : 'browser';
}

/** The implementation each target carries, beside `index.ts`. */
export const IMPLEMENTATION: Readonly<Record<BuildTarget, string>> = {
  browser: './browser.ts',
  desktop: './desktop.ts',
  mobile: './mobile.ts',
};

/**
 * The two areas the phone leaves out (ADR-0032, UC-SYN-07 check 7): `App.svelte` imports each
 * area's screens through one module, and the mobile build puts the page that names the web app in
 * its place, so no screen of either area is in the phone's bundle (`build/check-mobile-bundle.js`).
 */
export const LEFT_OUT_AREAS: readonly string[] = ['./views/AdministrationArea.svelte', './views/InstanceArea.svelte'];
/** What stands where a left-out area would be: the page that opens the web app. */
export const IN_WEB_APP = './views/InWebAppView.svelte';

/**
 * Where an import resolves for `target`: `index.ts`'s import of the browser implementation goes to
 * the target's, and every other import stays where it was. `undefined` means "unchanged".
 */
export function redirect(source: string, importer: string | undefined, target: BuildTarget): string | undefined {
  if (target === 'browser' || importer === undefined) return undefined;
  if (target === 'mobile' && LEFT_OUT_AREAS.includes(source) && /[\\/]src[\\/]App\.svelte$/.test(importer)) return IN_WEB_APP;
  if (source !== IMPLEMENTATION.browser) return undefined;
  if (!/[\\/]lib[\\/]platform[\\/]index\.ts$/.test(importer)) return undefined;
  return IMPLEMENTATION[target];
}
