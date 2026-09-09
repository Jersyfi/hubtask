// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Platform } from './index.ts';
import { browserStorage, tokenStore } from './tokenStore.ts';

/**
 * The browser: the primary target (ADR-0033), and the one with the narrowest promises — offline
 * here is a best-effort cache, never the offline guarantee, which belongs to the shells
 * (ADR-0031).
 *
 * The session pair lives in `sessionStorage`, and `tokenStore.ts` carries the reasoning: it
 * survives a reload, which F1-11 requires, and dies with the tab, which is all this target
 * promises. A shell holds its pair in the platform keystore instead, and this is the only file
 * that has to change for that (ADR-0031).
 */
const store = tokenStore(browserStorage());

export const platform: Platform = {
  target: 'browser',

  // Answering `undefined` rather than an empty string is the difference between "no credential"
  // and "a credential that is empty", and only one of those is ever true.
  bearer: () => store.read(),

  refreshToken: () => store.readRefresh(),

  holdSession: (pair) => store.write(pair),

  releaseBearer: () => store.clear(),

  // A navigation rather than a new window: the target answers `Content-Disposition: attachment`,
  // so the browser downloads and the page the reader was on stays where it was. A window opened
  // from an asynchronous callback is also the shape popup blockers refuse, and minting the target
  // when it is clicked makes the callback asynchronous by definition.
  openDownload: (url) => window.location.assign(url),

  // An object URL and a click, which is the only way a browser saves bytes it already has. The URL
  // is revoked immediately afterwards: it is a handle to memory, and one left behind keeps the
  // whole file alive for the life of the document.
  // `navigator.languages` is the ordered list the reader configured; `language` is the first of
  // them and is the fallback where the list is missing.
  preferredLanguages: () =>
    navigator.languages?.length ? [...navigator.languages] : [navigator.language].filter(Boolean),

  saveFile: (bytes, fileName) => {
    const url = URL.createObjectURL(bytes);
    const link = document.createElement('a');
    link.href = url;
    link.download = fileName;
    document.body.append(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
  },
};
