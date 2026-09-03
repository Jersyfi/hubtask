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
   * token is kept is exactly what differs between targets: the browser holds it for the tab's
   * lifetime, and the shells hold it in the platform keystore (ADR-0031). The engine asks per call
   * so that a refresh, or a sign-out, is seen by the next request rather than by the next reload.
   */
  bearer(): string | undefined;

  /**
   * Holds a bearer for the calls that follow.
   *
   * **Temporary, and the milestone that replaces it is named:** a personal access token typed by
   * the person using it is what F1 can honestly build, because `api/openapi.yaml` declares one
   * security scheme and no login route. Session management and the OIDC connection are `0.6.0`,
   * and they replace this method and everything behind it - the browser keeps its token where
   * script can reach it, and a session the browser holds instead is the point of that work.
   */
  holdBearer(token: string): void;

  /**
   * Forgets it. What sign-out calls, and what a `401` calls before returning to the token screen.
   *
   * The credential only; everything else the client holds is dropped by the caller
   * (`offline-sync.md` §9.6), because this seam knows about a token and not about a cache.
   */
  releaseBearer(): void;
}

export { platform } from './browser.ts';
