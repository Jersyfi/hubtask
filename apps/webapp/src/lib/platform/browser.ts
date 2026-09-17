// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { IndexedDbStorage, databaseNameFor } from '@hubtask/sync-engine';

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

/**
 * The browser's name, from what it says about itself. Coarse on purpose: this is a label a person
 * reads in a device list, not a fingerprint, and a browser that lies about itself is answered
 * with what it said.
 */
function browserName(): string {
  const agent = navigator.userAgent;
  const browser = /Firefox\//.test(agent) ? 'Firefox'
    : /Edg\//.test(agent) ? 'Edge'
    : /OPR\//.test(agent) ? 'Opera'
    : /Chrome\//.test(agent) ? 'Chrome'
    : /Safari\//.test(agent) ? 'Safari'
    : 'Browser';
  const system = /Windows/.test(agent) ? 'Windows'
    : /Android/.test(agent) ? 'Android'
    : /iPhone|iPad/.test(agent) ? 'iOS'
    : /Mac OS X/.test(agent) ? 'macOS'
    : /Linux/.test(agent) ? 'Linux'
    : undefined;
  return system ? `${browser} on ${system}` : browser;
}

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

  // The database is named after this origin and the account (ADR-0033 §4): the API is this
  // origin's (ADR-0028), so two installations on two hosts never share a copy, and two accounts on
  // one never do. A browser that refuses IndexedDB - a private window, a policy - answers nothing,
  // and the engine then runs online-only as it did before F6.
  storageFor: (accountId) => {
    try {
      return new IndexedDbStorage(databaseNameFor(window.location.origin, accountId));
    } catch {
      return undefined;
    }
  },

  deviceName: browserName,

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
