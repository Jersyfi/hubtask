// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { SessionPair, TokenStore } from './tokenStore.ts';

/**
 * The installed apps' session (ADR-0096, UC-SYN-07 check 3): the pair, the account and the
 * elevation, together with the workspace they belong to, held in the system's keystore through the
 * shell and in this page's memory - **never in the webview's storage**, which is a file any program
 * of the same user reads. The browser's `tokenStore` keeps the same shape for the tab instead.
 *
 * The keystore is written on every change and read once, when the app starts: a restart signs
 * nobody out, and the copy opens for the account the session names (UC-SYN-01 check 7).
 */

/** What the keystore holds, as one value. */
export interface HeldSession {
  /** The workspace's origin, `https://tasks.example.com`. */
  readonly server: string;
  readonly access: string;
  readonly refresh: string;
  readonly account?: string;
  readonly elevatedUntil?: string;
}

/** Reads the keystore's value; anything that is not a whole session is none. */
export function parseHeld(raw: unknown): HeldSession | undefined {
  if (typeof raw !== 'string' || raw === '') return undefined;
  let value: unknown;
  try {
    value = JSON.parse(raw);
  } catch {
    return undefined;
  }
  if (typeof value !== 'object' || value === null) return undefined;
  const held = value as Record<string, unknown>;
  const text = (key: string): string | undefined =>
    typeof held[key] === 'string' && held[key] !== '' ? (held[key] as string) : undefined;
  const server = text('server');
  const access = text('access');
  const refresh = text('refresh');
  if (server === undefined || access === undefined || refresh === undefined) return undefined;
  return {
    server,
    access,
    refresh,
    ...(text('account') !== undefined ? { account: text('account') } : {}),
    ...(text('elevatedUntil') !== undefined ? { elevatedUntil: text('elevatedUntil') } : {}),
  };
}

/** Where the keystore is reached: the shell's `session_keep` and `session_forget`. */
export interface SessionKeeper {
  keep(value: string): void;
  forget(): void;
}

/**
 * The `TokenStore` of an installed app: memory for every read, the keystore for every change.
 *
 * `server` is the workspace this run speaks to; a pair is held for it and nothing else.
 */
export function keystoreTokens(server: string, held: HeldSession | undefined, keeper: SessionKeeper): TokenStore {
  let current: HeldSession | undefined = held?.server === server ? held : undefined;

  const save = (next: HeldSession | undefined): void => {
    current = next;
    if (next === undefined) keeper.forget();
    else keeper.keep(JSON.stringify(next));
  };

  const withPair = (pair: SessionPair, keepElevation: boolean): HeldSession => ({
    server,
    access: pair.access,
    refresh: pair.refresh,
    ...(current?.account !== undefined ? { account: current.account } : {}),
    ...(keepElevation && current?.elevatedUntil !== undefined ? { elevatedUntil: current.elevatedUntil } : {}),
  });

  return {
    readAccount: () => current?.account,
    rememberAccount(id: string): void {
      if (current !== undefined && current.account !== id) save({ ...current, account: id });
    },
    read: () => current?.access,
    readRefresh: () => current?.refresh,
    // A new session's pair belongs to whoever signed in now, so the account is learnt again.
    write(pair: SessionPair): void {
      save({ server, access: pair.access, refresh: pair.refresh });
    },
    rotate(pair: SessionPair): void {
      save(withPair(pair, true));
    },
    readElevatedUntil: () => current?.elevatedUntil,
    rememberElevatedUntil(until: string): void {
      if (current !== undefined) save({ ...current, elevatedUntil: until });
    },
    clear(): void {
      save(undefined);
    },
  };
}

/**
 * A workspace the app holds (UC-SYN-07 check 4, UC-ID-25 check 7): its address, its host, its name
 * and the host of the installation it belongs to. No credential, so in the webview's own storage;
 * its session is in the keystore under `server`. Signing out keeps it, so *Sign in again* needs no
 * address (the approved preview's choice); only *Forget this workspace* removes it.
 */
export interface AppWorkspace {
  readonly server: string;
  readonly host: string;
  readonly name?: string;
  /**
   * The installation's own host, which the list groups by (the approved preview's choice); the
   * workspace's own host where the server names none - an installation in single mode.
   */
  readonly installation?: string;
}

/** The workspace this window is open on. */
export const WORKSPACE_KEY = 'hubtask.app.workspace';

/** Every workspace the app holds, in the order they were added. */
export const WORKSPACES_KEY = 'hubtask.app.workspaces';

/**
 * The workspaces forgotten while their server could not be reached: their session stays in the
 * keystore until the grant is withdrawn there (the approved preview's choice), by the next start
 * that reaches it.
 */
export const OWED_KEY = 'hubtask.app.owed';

/** Set by the sign-in before the window starts on the new session, taken once by the toast. */
export const ARRIVAL_KEY = 'hubtask.app.arrived';

/** More workspaces than any person holds; the list keeps no more. */
const WORKSPACES_MAX = 50;

export interface PlainStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

function workspaceOf(value: unknown): AppWorkspace | undefined {
  if (typeof value !== 'object' || value === null) return undefined;
  const held = value as Partial<Record<keyof AppWorkspace, unknown>>;
  if (typeof held.server !== 'string' || typeof held.host !== 'string') return undefined;
  return {
    server: held.server,
    host: held.host,
    ...(typeof held.name === 'string' && held.name !== '' ? { name: held.name } : {}),
    ...(typeof held.installation === 'string' && held.installation !== '' ? { installation: held.installation } : {}),
  };
}

function readJson(storage: PlainStorage | undefined, key: string): unknown {
  try {
    return JSON.parse(storage?.getItem(key) ?? 'null');
  } catch {
    return null;
  }
}

function writeJson(storage: PlainStorage | undefined, key: string, value: unknown): void {
  try {
    if (value === undefined) storage?.removeItem(key);
    else storage?.setItem(key, JSON.stringify(value));
  } catch {
    // A webview that refuses storage asks for the address again next time; no credential is lost.
  }
}

/** The workspace this window is open on, or none. */
export function readWorkspace(storage: PlainStorage | undefined): AppWorkspace | undefined {
  return workspaceOf(readJson(storage, WORKSPACE_KEY));
}

/** Opens the window on `workspace` from its next start; nothing removes the current one. */
export function writeWorkspace(storage: PlainStorage | undefined, workspace: AppWorkspace | undefined): void {
  writeJson(storage, WORKSPACE_KEY, workspace);
}

/**
 * The workspaces the app holds, one per server. A current workspace the list does not name - a
 * window signed in before the app held several - is the list's first.
 */
export function readWorkspaces(storage: PlainStorage | undefined): readonly AppWorkspace[] {
  const raw = readJson(storage, WORKSPACES_KEY);
  const seen = new Set<string>();
  const list: AppWorkspace[] = [];
  for (const each of Array.isArray(raw) ? raw : []) {
    const workspace = workspaceOf(each);
    if (workspace === undefined || seen.has(workspace.server)) continue;
    seen.add(workspace.server);
    list.push(workspace);
  }
  const current = readWorkspace(storage);
  if (current !== undefined && !seen.has(current.server)) list.unshift(current);
  return list.slice(0, WORKSPACES_MAX);
}

/** Keeps a workspace in the list, in place of the one for its server, where that one stood. */
export function keepWorkspace(storage: PlainStorage | undefined, workspace: AppWorkspace): void {
  const list = [...readWorkspaces(storage)];
  const at = list.findIndex((each) => each.server === workspace.server);
  if (at >= 0) list[at] = { ...list[at], ...workspace };
  else list.push(workspace);
  writeJson(storage, WORKSPACES_KEY, list.slice(0, WORKSPACES_MAX));
}

/** Takes a workspace off the list. */
export function dropWorkspace(storage: PlainStorage | undefined, server: string): void {
  const list = readWorkspaces(storage).filter((each) => each.server !== server);
  writeJson(storage, WORKSPACES_KEY, list.length === 0 ? undefined : list);
}

/** The installations the list holds, each with its workspaces, in the list's order. */
export function groupedByInstallation(
  list: readonly AppWorkspace[],
): readonly { readonly installation: string; readonly workspaces: readonly AppWorkspace[] }[] {
  const groups = new Map<string, AppWorkspace[]>();
  for (const each of list) {
    const key = each.installation ?? each.host;
    groups.set(key, [...(groups.get(key) ?? []), each]);
  }
  return [...groups].map(([installation, workspaces]) => ({ installation, workspaces }));
}

/** The servers whose grant is still to be withdrawn. */
export function owedRevocations(storage: PlainStorage | undefined): readonly string[] {
  const raw = readJson(storage, OWED_KEY);
  return Array.isArray(raw) ? raw.filter((each): each is string => typeof each === 'string') : [];
}

function writeOwed(storage: PlainStorage | undefined, servers: readonly string[]): void {
  writeJson(storage, OWED_KEY, servers.length === 0 ? undefined : [...new Set(servers)]);
}

/** What the server answered when the app ended its session there. */
export type Ending = 'ended' | 'unreachable';

/** Where a workspace's session and copy are, as forgetting one needs them. */
export interface ForgetSteps {
  readonly storage: PlainStorage | undefined;
  /** The session the keystore holds for a server, or none. */
  readSession(server: string): Promise<HeldSession | undefined>;
  forgetSession(server: string): Promise<void>;
  /** Files a session again: one stopping the open workspace removed, kept while its grant is owed. */
  keepSession(held: HeldSession): Promise<void>;
  /** The copy of that account at that server, and its key. */
  forgetCopy(server: string, account: string): Promise<void>;
  /** Ends the session at its server, which withdraws the app's grant (identity.md §15.3). */
  revoke(held: HeldSession): Promise<Ending>;
  /**
   * Everything this window holds of the open workspace, ended without asking its server - the
   * engine, the stream, what it remembered - before its copy goes.
   */
  stopHere(): Promise<void>;
}

/**
 * *Forget this workspace* (UC-SYN-07 check 4, UC-ID-25 check 7): its grant withdrawn, its session, copy and
 * key deleted, and it leaves the list - nothing of another workspace is touched. Where its server
 * cannot be reached, the copy and key go at once and the grant is withdrawn by a later start
 * (`settleOwed`). Answers the workspace the window opens on next where the one forgotten was open:
 * the next in the list, or none, which is *Which workspace?*.
 */
export async function forgetWorkspace(
  steps: ForgetSteps,
  server: string,
): Promise<{ readonly wasOpen: boolean; readonly next: AppWorkspace | undefined }> {
  const list = readWorkspaces(steps.storage);
  const wasOpen = readWorkspace(steps.storage)?.server === server;
  const at = list.findIndex((each) => each.server === server);
  const held = await steps.readSession(server);
  // The grant first, while the window still works: a person waits on the dialog, not a blank card.
  const ending: Ending = held === undefined ? 'ended' : await steps.revoke(held);
  if (wasOpen) await steps.stopHere();
  if (ending === 'unreachable' && held !== undefined) {
    writeOwed(steps.storage, [...owedRevocations(steps.storage), server]);
    await steps.keepSession(held);
  } else {
    await steps.forgetSession(server);
  }
  if (held?.account !== undefined) await steps.forgetCopy(server, held.account);
  dropWorkspace(steps.storage, server);
  const rest = list.filter((each) => each.server !== server);
  const next = wasOpen ? (rest[Math.max(0, at)] ?? rest[0]) : undefined;
  if (wasOpen) writeWorkspace(steps.storage, next);
  return { wasOpen, next };
}

/**
 * The grants still owed a withdrawal, withdrawn where their server answers now - unless the
 * workspace was added again meanwhile, whose new session is not the one forgotten.
 */
export async function settleOwed(
  steps: Pick<ForgetSteps, 'storage' | 'readSession' | 'forgetSession' | 'revoke'>,
): Promise<void> {
  const held = new Set(readWorkspaces(steps.storage).map((each) => each.server));
  const still: string[] = [];
  for (const server of owedRevocations(steps.storage)) {
    if (held.has(server)) continue;
    const session = await steps.readSession(server);
    if (session !== undefined && (await steps.revoke(session)) === 'unreachable') {
      still.push(server);
      continue;
    }
    await steps.forgetSession(server);
  }
  writeOwed(steps.storage, still);
}
