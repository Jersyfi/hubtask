// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { invoke } from '@tauri-apps/api/core';

import type { Platform } from './index.ts';
import { pageLanguages, saveByLink } from './page.ts';
import { LOCAL_SERVER, apiBaseOf } from './server.ts';
import { browserStorage, tokenStore } from './tokenStore.ts';
import { entryList, type DeviceWorkspaces, type ListStorage } from './workspaces.ts';

/**
 * The desktop shell (ADR-0031): the web app inside the Tauri window on Windows and Linux,
 * chosen by `vite build --mode desktop` (`target.ts`). Only what differs from the page lives here.
 *
 * What this target does not do yet: the server is a build's (`VITE_HUBTASK_SERVER`, the local one
 * by default) until the person is asked for it (ADR-0096); the pair lives as the browser's does
 * until it moves into the keystore (UC-SYN-07/3); there is no local copy until the encrypted one
 * (UC-SYN-01/7), because an unencrypted copy is exactly what the shells promise not to keep
 * (ADR-0033 §4); the workspace list stays on this device without a switch (UC-ID-25/7).
 */

const server = import.meta.env.VITE_HUBTASK_SERVER ?? LOCAL_SERVER;
const apiBase = apiBaseOf(server);
if (apiBase === undefined) throw new Error('the shell was built for a server that is not one');

/** What the system says about this device, asked once before anything is drawn. */
interface SystemIdentity {
  readonly name: string;
  readonly platform: string;
}
const identity = await invoke<SystemIdentity>('system_identity').catch(
  (): SystemIdentity => ({ name: 'Hubtask', platform: 'desktop' }),
);

const store = tokenStore(browserStorage());

function localList(): ListStorage | undefined {
  try {
    return globalThis.localStorage;
  } catch {
    return undefined;
  }
}
const list = entryList(localList());

// The list without its switch (UC-ID-25/7): kept on this device, never moved through an address.
const workspaces: DeviceWorkspaces = {
  list: () => list.all(),
  drop: (entry) => list.drop(entry),
  keep: (entry) => list.keep(entry),
  remember: (entry) => list.keep(entry),
  forget: (host) => list.forget(host),
  open: () => {},
  takeHop: () => undefined,
  leave: () => {},
};

export const platform: Platform = {
  target: 'desktop',
  apiBase,
  bearer: () => store.read(),
  refreshToken: () => store.readRefresh(),
  holdSession: (pair) => store.write(pair),
  renewSession: (pair) => store.rotate(pair),
  releaseBearer: () => store.clear(),
  // The webview hands a navigation to an attachment to the system's download.
  openDownload: (url) => window.location.assign(url),
  saveFile: saveByLink,
  preferredLanguages: pageLanguages,
  storageFor: () => undefined,
  rememberAccount: (id) => store.rememberAccount(id),
  lastAccount: () => store.readAccount(),
  rememberElevation: (until) => store.rememberElevatedUntil(until),
  elevatedUntil: () => store.readElevatedUntil(),
  deviceName: () => identity.name,
  devicePlatform: identity.platform,
  workspaces,
};
