// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { SqliteStorage, copyNameFor, type ShellCall } from '@hubtask/sync-engine';
import { getVersion } from '@tauri-apps/api/app';
import { invoke } from '@tauri-apps/api/core';
import { listen } from '@tauri-apps/api/event';

import {
  ARRIVAL_KEY,
  forgetWorkspace,
  keepWorkspace,
  keystoreTokens,
  owedRevocations,
  parseHeld,
  readWorkspace,
  readWorkspaces,
  settleOwed,
  writeWorkspace,
  type AppWorkspace,
  type ForgetSteps,
  type HeldSession,
  type SessionKeeper,
} from './appSession.ts';
import type { AppUpdates, AppWorkspaces, BrowserSignIn, Platform, ReturnedCode } from './index.ts';
import { pageLanguages, saveByLink } from './page.ts';
import { LOCAL_SERVER, apiBaseOf } from './server.ts';
import { loopbackRedirect } from '../signin/appSignIn.ts';
import { fromShell, productVersion } from './update.ts';
import { entryList, type DeviceWorkspaces, type ListStorage } from './workspaces.ts';

/**
 * The installed apps' shell (ADR-0031): the web app inside the Tauri window, what the desktop
 * (`desktop.ts`) and the Android app (`mobile.ts`) share. Only what differs from the page lives here.
 *
 * The app signs in through the system browser (ADR-0096) and holds any number of workspaces, of
 * one installation or several (UC-SYN-07 check 4, UC-ID-25 check 7), each with its own session in
 * the system's keystore (`appSession.ts`) and its own copy - the shell's SQLite, encrypted under a
 * key in the keystore (ADR-0033 §4), one per server and account, which holds a file captured offline
 * until it is uploaded. A window speaks to one workspace: the one it was last open on, a build's
 * (`VITE_HUBTASK_SERVER`, the local one by default) only before the first sign-in. Switching starts
 * the window again on the other one's session and copy.
 */

function localList(): ListStorage | undefined {
  try {
    return globalThis.localStorage;
  } catch {
    return undefined;
  }
}

/** What the system says about this device, asked once before anything is drawn. */
interface SystemIdentity {
  readonly name: string;
  readonly platform: string;
}
const identity = await invoke<SystemIdentity>('system_identity').catch(
  (): SystemIdentity => ({ name: 'Hubtask', platform: 'desktop' }),
);
const appVersion = await getVersion().catch(() => '0.0.0');

/** The session the keystore holds for a workspace; a value that is not one is none. */
async function sessionOf(server: string): Promise<HeldSession | undefined> {
  const held = parseHeld(await invoke<string | null>('session_read', { server }).catch(() => undefined));
  return held?.server === server ? held : undefined;
}

let workspace = readWorkspace(localList());
const held = workspace === undefined ? undefined : await sessionOf(workspace.server);
const server = workspace?.server ?? import.meta.env.VITE_HUBTASK_SERVER ?? LOCAL_SERVER;
const apiBase = apiBaseOf(server);
if (apiBase === undefined) throw new Error('the shell was given a server that is not one');

/**
 * The keystore's writes, one after the other: a refresh's pair must never be overtaken by the
 * write before it, or a restart would present a retired refresh token.
 */
let writes: Promise<unknown> = Promise.resolve();
/** The server the window speaks to: its origin, which files its session and names its copy. */
const origin = new URL(apiBase).origin;
const keeper: SessionKeeper = {
  keep: (value) => {
    writes = writes.then(() => invoke('session_keep', { server: origin, value })).catch(() => {});
  },
  forget: () => {
    writes = writes.then(() => invoke('session_forget', { server: origin })).catch(() => {});
  },
};
const store = keystoreTokens(origin, held, keeper);

const shell: ShellCall = (command, args) => invoke(command, args);
const copyAt = (at: string, accountId: string) => new SqliteStorage(copyNameFor(at, accountId), shell);
const copyOf = (accountId: string) => copyAt(origin, accountId);

const list = entryList(localList());

// The browser's list of signed entries, kept without its switch: the app's own list is `appWorkspaces` (UC-ID-25/7).
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

/** Where forgetting finds a workspace's session and copy: the keystore and the shell's copies. */
function forgetSteps(revoke: ForgetSteps['revoke'], stopHere: ForgetSteps['stopHere']): ForgetSteps {
  return {
    storage: localList(),
    readSession: async (at) => {
      // The open workspace's last write first, or a rotated pair would be read as it was before.
      await writes;
      return sessionOf(at);
    },
    // After the open workspace's own last write, which stopping it queued.
    forgetSession: async (at) => {
      writes = writes.then(() => invoke('session_forget', { server: at })).catch(() => {});
      await writes;
    },
    keepSession: async (session) => {
      writes = writes.then(() => invoke('session_keep', { server: session.server, value: JSON.stringify(session) })).catch(() => {});
      await writes;
    },
    forgetCopy: (at, account) => copyAt(at, account).forget().catch(() => {}),
    revoke,
    stopHere,
  };
}

/** Starts the window again: on the workspace now current, from its own session and copy. */
function restart(): void {
  window.location.reload();
}

const appWorkspaces: AppWorkspaces = {
  get current() {
    return workspace;
  },
  list: () => readWorkspaces(localList()),
  isSignedIn: async (at) => (at === origin ? store.read() !== undefined : (await sessionOf(at)) !== undefined),
  async waiting(at) {
    const account = at === origin ? store.readAccount() : (await sessionOf(at))?.account;
    if (account === undefined) return 0;
    return (await copyAt(at, account).all('queue').catch(() => [])).length;
  },
  open(at) {
    const next = readWorkspaces(localList()).find((each) => each.server === at);
    if (next === undefined || at === origin) return;
    writeWorkspace(localList(), next);
    restart();
  },
  async forget(at, revoke, stopHere) {
    const { wasOpen } = await forgetWorkspace(forgetSteps(revoke, stopHere), at);
    if (wasOpen) {
      await writes;
      restart();
    }
  },
  async settle(revoke) {
    if (owedRevocations(localList()).length === 0) return;
    await settleOwed(forgetSteps(revoke, async () => {}));
  },
};

function arrived(): boolean {
  try {
    const value = globalThis.sessionStorage.getItem(ARRIVAL_KEY) === '1';
    globalThis.sessionStorage.removeItem(ARRIVAL_KEY);
    return value;
  } catch {
    return false;
  }
}
let arrival = arrived();

let onReturned: ((answer: ReturnedCode) => void) | undefined;
/** Where the next sign-in's or step-up's answer goes, once: the listener's or the deep link's. */
export function awaitReturn(returned: (answer: ReturnedCode) => void): void {
  onReturned = returned;
}
void listen<ReturnedCode>('signin-returned', (event) => {
  const deliver = onReturned;
  onReturned = undefined;
  deliver?.(event.payload);
}).catch(() => {});

export const shellSignIn: BrowserSignIn = {
  get workspace() {
    return workspace;
  },
  appVersion,
  appPlatform: identity.platform === 'windows' || identity.platform === 'android' ? identity.platform : 'linux',
  async listen(page, returned) {
    awaitReturn(returned);
    return invoke<number>('signin_listen', { title: page.title, body: page.body, lang: page.lang, style: page.style ?? '', mark: page.mark ?? '' });
  },
  async listenForSignIn(page, returned) {
    return loopbackRedirect(await shellSignIn.listen(page, returned));
  },
  retire() {
    onReturned = undefined;
    void invoke('signin_retire').catch(() => {});
  },
  async openInBrowser(url) {
    await invoke('open_in_browser', { url });
  },
  async enter(next: AppWorkspace, pair) {
    // Straight to the keystore and awaited, because the window starts again on what it holds.
    await writes;
    await invoke('session_keep', { server: next.server, value: JSON.stringify({ server: next.server, access: pair.access, refresh: pair.refresh }) });
    // Added, or signed in again: the list keeps its name until the workspace says it afresh, and the
    // window opens on it (the approved preview's choice).
    const known = readWorkspaces(localList()).find((each) => each.server === next.server);
    const named = { ...(known?.name !== undefined ? { name: known.name } : {}), ...next };
    keepWorkspace(localList(), named);
    writeWorkspace(localList(), named);
    try {
      globalThis.sessionStorage.setItem(ARRIVAL_KEY, '1');
    } catch {
      // No toast, then; the sync line says the rest.
    }
    window.location.reload();
  },
  nameWorkspace(name) {
    if (workspace === undefined || workspace.name === name) return;
    workspace = { ...workspace, name };
    writeWorkspace(localList(), workspace);
    keepWorkspace(localList(), workspace);
  },
  takeArrival() {
    const value = arrival;
    arrival = false;
    return value;
  },
};

/** The desktop's updater: the shell checks the manifest and the package, the page asks and offers. */
const updates: AppUpdates = {
  current: productVersion(appVersion),
  offersOn: 'every-page',
  laterLasts: 'start',
  check: async () => fromShell(await invoke<Record<string, string>>('update_check').catch(() => ({}))),
  install: () => invoke('update_install'),
  open: (url) => invoke('open_in_browser', { url }),
};

/** The desktop's seam, which the Android app's starts from. */
export const shellPlatform: Platform = {
  target: 'desktop',
  apiBase,
  webOrigin: origin,
  bearer: () => store.read(),
  refreshToken: () => store.readRefresh(),
  holdSession: (pair) => store.write(pair),
  renewSession: (pair) => store.rotate(pair),
  releaseBearer: () => store.clear(),
  // The webview hands a navigation to an attachment to the system's download.
  openDownload: (url) => window.location.assign(url),
  saveFile: saveByLink,
  preferredLanguages: pageLanguages,
  storageFor: copyOf,
  holdsFiles: true,
  firstSynchronisation: 'pages',
  forgetCopy: (accountId) => copyOf(accountId).forget(),
  rememberAccount: (id) => store.rememberAccount(id),
  lastAccount: () => store.readAccount(),
  rememberElevation: (until) => store.rememberElevatedUntil(until),
  elevatedUntil: () => store.readElevatedUntil(),
  deviceName: () => identity.name,
  devicePlatform: identity.platform,
  workspaces,
  browserSignIn: shellSignIn,
  appWorkspaces,
  updates,
};
