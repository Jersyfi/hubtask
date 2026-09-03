// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Platform } from './index.ts';
import { browserStorage, tokenStore } from './tokenStore.ts';

/**
 * The browser: the primary target (ADR-0033), and the one with the narrowest promises — offline
 * here is a best-effort cache, never the offline guarantee, which belongs to the shells
 * (ADR-0031).
 *
 * The token lives in `sessionStorage`, and `tokenStore.ts` carries the reasoning: it survives a
 * reload, which F1-11 requires, and dies with the tab, which is all this target promises. A shell
 * holds its token in the platform keystore instead, and this is the only file that has to change
 * for that (ADR-0031).
 */
const store = tokenStore(browserStorage());

export const platform: Platform = {
  target: 'browser',

  // Answering `undefined` rather than an empty string is the difference between "no credential"
  // and "a credential that is empty", and only one of those is ever true.
  bearer: () => store.read(),

  holdBearer: (token) => store.write(token),

  releaseBearer: () => store.clear(),
};
