// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The device's list of workspaces (UC-ID-25, ADR-0095 §7, multi-tenancy.md §3.1): the model every
 * target shares, and the shapes the browser moves it in.
 *
 * **An entry is what the installation signed, kept as it was signed.** `<payload>.<tag>`, both
 * unpadded base64url; the payload is `{"host","name"}` and nothing else. The list stores the signed
 * strings only - no address of a person, no token - and reads host and name out of them to draw a
 * row. Whether an entry is the installation's, and whether its host still answers, is the server's
 * answer (`POST /auth/device-list:check`), never this module's: it cannot check a signature.
 *
 * **Where the list lives differs by target**, which is why this sits behind the platform seam. The
 * browser keeps it in the `localStorage` of the installation's own address and hands a copy to a
 * workspace's address whenever it sends the browser there (`#workspaces=`); every change goes
 * through the installation's address, by a redirect there and back. A shell keeps one list per
 * server in its own store and needs no redirect (UC-ID-25 check 7): it implements `DeviceWorkspaces` over the
 * same `readEntry` and `entryList`.
 */

/** One workspace of the list, as a row draws it. */
export interface KnownWorkspace {
  /** The entry as the installation signed it; what is stored and what the check is asked about. */
  readonly entry: string;
  /** The workspace's main address, a host without a scheme. */
  readonly host: string;
  readonly name: string;
}

/** Where the list is kept: the little of `Storage` this needs, so a test can hand it something else. */
export interface ListStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

/** The key the list is kept under, at the installation's address and as a workspace's copy. */
export const LIST_KEY = 'hubtask.workspaces';

/** A list longer than this is no person's; the server checks no more at once. */
export const LIST_MAX = 50;

const ENTRY = /^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/;
const HOST = /^[a-z0-9.-]+$/;

function decodeBase64Url(text: string): string | undefined {
  try {
    const padded = text.replaceAll('-', '+').replaceAll('_', '/');
    const binary = atob(padded + '='.repeat((4 - (padded.length % 4)) % 4));
    return new TextDecoder('utf-8', { fatal: true }).decode(Uint8Array.from(binary, (c) => c.charCodeAt(0)));
  } catch {
    return undefined;
  }
}

/**
 * What an entry says, or nothing for a string that is not shaped as one. A payload that holds
 * anything beside the host and the name is not an entry: the list holds those two and no more.
 */
export function readEntry(entry: string): KnownWorkspace | undefined {
  if (entry.length > 1024 || !ENTRY.test(entry)) return undefined;
  const json = decodeBase64Url(entry.slice(0, entry.indexOf('.')));
  if (json === undefined) return undefined;
  let payload: unknown;
  try {
    payload = JSON.parse(json);
  } catch {
    return undefined;
  }
  if (typeof payload !== 'object' || payload === null || Array.isArray(payload)) return undefined;
  const keys = Object.keys(payload).sort();
  if (keys.length !== 2 || keys[0] !== 'host' || keys[1] !== 'name') return undefined;
  const { host, name } = payload as { host: unknown; name: unknown };
  if (typeof host !== 'string' || !HOST.test(host) || typeof name !== 'string' || name.trim() === '') return undefined;
  return { entry, host, name };
}

/** The list over a store: the entries, newest first, one per host. */
export interface EntryList {
  all(): readonly KnownWorkspace[];
  /** Keeps an entry, in place of one for the same host. */
  keep(entry: string): void;
  /** Removes the entries for a host. */
  forget(host: string): void;
  /** Removes exactly this entry - one the installation did not sign. */
  drop(entry: string): void;
  /** Replaces the whole list with a copy handed over. */
  replace(entries: readonly string[]): void;
}

/**
 * The list kept under `LIST_KEY`. Every access is guarded: a browser that refuses storage keeps no
 * list, which is a device that remembers nothing - never one that cannot sign in.
 */
export function entryList(storage: ListStorage | undefined): EntryList {
  function read(): KnownWorkspace[] {
    let raw: string | null = null;
    try {
      raw = storage?.getItem(LIST_KEY) ?? null;
    } catch {
      return [];
    }
    let parsed: unknown;
    try {
      parsed = raw ? JSON.parse(raw) : [];
    } catch {
      return [];
    }
    if (!Array.isArray(parsed)) return [];
    const seen = new Set<string>();
    const kept: KnownWorkspace[] = [];
    for (const each of parsed) {
      const known = typeof each === 'string' ? readEntry(each) : undefined;
      if (!known || seen.has(known.host)) continue;
      seen.add(known.host);
      kept.push(known);
    }
    return kept.slice(0, LIST_MAX);
  }
  function write(list: readonly KnownWorkspace[]): void {
    try {
      if (list.length === 0) storage?.removeItem(LIST_KEY);
      else storage?.setItem(LIST_KEY, JSON.stringify(list.slice(0, LIST_MAX).map((each) => each.entry)));
    } catch {
      // A browser that refuses storage keeps the list for nobody; the next sign-in hands it over again.
    }
  }
  return {
    all: read,
    keep(entry) {
      const known = readEntry(entry);
      if (!known) return;
      write([known, ...read().filter((each) => each.host !== known.host)]);
    },
    forget(host) {
      write(read().filter((each) => each.host !== host));
    },
    drop(entry) {
      write(read().filter((each) => each.entry !== entry));
    },
    replace(entries) {
      write(entries.map(readEntry).filter((each): each is KnownWorkspace => each !== undefined));
    },
  };
}

/** The address a host opens at: this page's scheme and port, which every workspace shares. */
export function addressOf(host: string, here: { readonly protocol: string; readonly port: string }): string {
  return `${here.protocol}//${host}${here.port ? `:${here.port}` : ''}`;
}

/** A path to come back to: one of this origin's, never another's. */
function ownPath(path: string | null): string {
  return path && path.startsWith('/') && !path.startsWith('//') && !path.startsWith('/\\') ? path : '/';
}

/** The path the installation's address takes a change of the list at. */
export const HOP_PATH = '/workspaces';

/** What a redirect to the installation's address asks of the list. */
export type Hop =
  | { readonly kind: 'add'; readonly entry: string; readonly path: string }
  | { readonly kind: 'forget'; readonly host: string; readonly back: string; readonly path: string }
  | { readonly kind: 'open'; readonly host: string };

/** The fragment a hop is carried in: never sent to a server, and removed once read. */
export function hopFragment(hop: Hop): string {
  const fields = new URLSearchParams();
  if (hop.kind === 'add') {
    fields.set('add', hop.entry);
    fields.set('path', hop.path);
  } else if (hop.kind === 'forget') {
    fields.set('forget', hop.host);
    fields.set('back', hop.back);
    fields.set('path', hop.path);
  } else {
    fields.set('open', hop.host);
  }
  return `#${fields.toString()}`;
}

/** Reads a hop out of a fragment, or nothing. */
export function readHop(hash: string): Hop | undefined {
  const fields = new URLSearchParams(hash.replace(/^#/, ''));
  const add = fields.get('add');
  if (add) return { kind: 'add', entry: add, path: ownPath(fields.get('path')) };
  const forget = fields.get('forget');
  const back = fields.get('back');
  if (forget && back) return { kind: 'forget', host: forget, back, path: ownPath(fields.get('path')) };
  const open = fields.get('open');
  if (open) return { kind: 'open', host: open };
  return undefined;
}

/** The fragment that hands a workspace's address its copy of the list. */
export function handoverFragment(entries: readonly string[]): string {
  return `#${new URLSearchParams({ workspaces: entries.join(',') }).toString()}`;
}

/** The copy a fragment hands over, or nothing where it hands none. */
export function readHandover(hash: string): string[] | undefined {
  const fields = new URLSearchParams(hash.replace(/^#/, ''));
  const list = fields.get('workspaces');
  if (list === null) return undefined;
  return list.split(',').filter((each) => each !== '');
}

/**
 * The device's list as a target offers it. The browser's moves the browser through the
 * installation's address; a shell's keeps the list itself.
 */
export interface DeviceWorkspaces {
  /** The list as this device holds it here. */
  list(): readonly KnownWorkspace[];
  /** Removes an entry the installation did not sign, here only. */
  drop(entry: string): void;
  /** Keeps an entry the installation confirmed, where the list lives. */
  keep(entry: string): void;
  /** After a sign-in: keeps this workspace's entry where the list lives, and comes back to `path`. */
  remember(entry: string, installation: string, path: string): void;
  /**
   * Forgets a workspace on this device at once, and where the list lives; `back` is the entry of
   * the workspace this is asked from, which the browser comes back to.
   */
  forget(host: string, installation: string, back: string | undefined, path: string): void;
  /** Opens a workspace of the list, which signs in on its own. */
  open(host: string, installation: string): void;
  /**
   * Where the list lives: the change a redirect brought, taken once, or nothing. Only the browser
   * is ever sent to the installation's address with one.
   */
  takeHop(): Hop | undefined;
  /** Leaves the list's home for a workspace of it, at `path`, handing over a copy. */
  leave(host: string, path: string): void;
}
