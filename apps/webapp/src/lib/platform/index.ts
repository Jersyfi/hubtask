// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The platform seam of ADR-0033: everything conditioned on where this bundle runs lives behind
 * this interface, so no `isTauri` conditional ever lands in a component. The implementation is
 * chosen at build time — today there is only the browser; the Tauri implementation arrives with
 * the shells (ADR-0031) and is selected by the shell's build, not by runtime sniffing.
 *
 * The interface is deliberately as small as what exists. The ports ADR-0033 already names for
 * later — local persistence, the keystore-held encryption key — are added here when the
 * sync-engine and shell work packages bring their first real implementation, not before
 * (CLAUDE.md: no speculative abstractions).
 */
export interface Platform {
  /**
   * Which target this bundle serves: `browser` now, `desktop` and `mobile` with the Tauri
   * shells. A capability difference between targets is expressed as a field on this interface,
   * never as a check on this value scattered through components.
   */
  readonly target: 'browser' | 'desktop' | 'mobile';

  /**
   * The bearer for an API call, or `undefined` when nobody is signed in.
   *
   * A function rather than a value, and here rather than in the sync engine, because *where* a
   * token is kept is exactly what differs between targets: the browser holds it in memory for the
   * tab's lifetime, and the shells hold it in the platform keystore (ADR-0031). The engine asks
   * per call so that a refresh, or a sign-out, is seen by the next request rather than by the
   * next reload.
   *
   * F1-11 puts a real token behind it. Until then it answers `undefined`, which is the honest
   * state of an application nobody has signed into.
   */
  bearer(): string | undefined;
}

export { platform } from './browser.ts';
