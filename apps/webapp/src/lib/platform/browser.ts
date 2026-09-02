// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Platform } from './index.ts';

/**
 * The browser: the primary target (ADR-0033), and the one with the narrowest promises — offline
 * here is a best-effort cache, never the offline guarantee, which belongs to the shells
 * (ADR-0031).
 */
export const platform: Platform = {
  target: 'browser',

  // Nobody is signed in yet: F1-11 brings the token and the place it is kept. Answering
  // `undefined` rather than an empty string is the difference between "no credential" and "a
  // credential that is empty", and only one of those is true.
  bearer: () => undefined,
};
