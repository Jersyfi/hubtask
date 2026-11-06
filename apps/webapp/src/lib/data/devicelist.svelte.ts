// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The device's list of workspaces as the screens use it (UC-ID-25, ADR-0095 §7): the account
 * menu's *Switch workspace*, and the installation's own address signed out.
 *
 * Where the list lives is the platform's (`platform/workspaces.ts`); what is asked of the server is
 * here. After a sign-in the workspace's signed entry is fetched and taken to where the list lives;
 * before the installation's address keeps an entry, and whenever a list is shown, the server says
 * which entries it signed and whether each host still answers. An entry it did not sign is dropped
 * without a word - it was never this installation's (check 5).
 */

import type { DeviceListChecked, DeviceListEntry } from '@hubtask/sync-engine';

import { platform, type KnownWorkspace } from '../platform/index.ts';
import { HOP_PATH } from '../platform/workspaces.ts';
import { manifest } from './capabilities.svelte.ts';
import { engine } from './engine.ts';

const ENTRY = '/tenant/device-list-entry';
const CHECK = '/auth/device-list:check';

/** A row of the list: the entry, and whether its host still answers - unknown until asked. */
export interface ListedWorkspace extends KnownWorkspace {
  readonly answers: boolean | undefined;
}

/** What the server said of an entry it signed. */
interface Checked {
  readonly answers: boolean;
  readonly host: string;
}

/**
 * This workspace's entry, read once; nothing where it cannot be had. A read rather than a
 * subscription: the entry is handed on at once, and nothing on a screen follows it.
 */
function readOwnEntry(): Promise<string | undefined> {
  return new Promise((resolve) => {
    let settled = false;
    let stop: (() => void) | undefined;
    const settle = (entry: string | undefined) => {
      if (settled) return;
      settled = true;
      resolve(entry);
      queueMicrotask(() => stop?.());
    };
    stop = engine.subscribe<DeviceListEntry>({ path: ENTRY }, (state) => {
      if (state.status === 'ready') settle(state.data.entry);
      else if (state.status === 'failed') settle(undefined);
    });
    if (settled) stop();
  });
}

class DeviceList {
  #entries = $state<readonly KnownWorkspace[]>([]);
  #answers = $state<ReadonlyMap<string, Checked>>(new Map());

  /** The installation's own address, in multi mode; nothing in single mode, which keeps no list. */
  get installation(): string | undefined {
    if (!manifest.isMultiTenant) return undefined;
    return manifest.value?.installation_url ?? undefined;
  }

  /** Whether this page is at the installation's own address, where the list lives. */
  get isHome(): boolean {
    const installation = this.installation;
    if (installation === undefined) return false;
    try {
      return new URL(installation).host === location.host;
    } catch {
      return false;
    }
  }

  /**
   * Whether this page may be the list's home taking a redirect, while the manifest has not said:
   * then nothing is drawn rather than a sign-in card for a moment.
   */
  get mayBeHome(): boolean {
    return !manifest.isRead && manifest.failure === undefined && location.pathname === HOP_PATH;
  }

  /** The rows, as this device holds them. */
  get rows(): readonly ListedWorkspace[] {
    return this.#entries.map((each) => ({ ...each, answers: this.#answers.get(each.entry)?.answers }));
  }

  /**
   * Reads the list and asks which entries still answer (the stale entry is checked when the list
   * is shown - the approved preview). A server that cannot be reached leaves every row as it is.
   */
  async show(): Promise<void> {
    this.#entries = platform.workspaces.list();
    const answers = await this.#check(this.#entries.map((each) => each.entry));
    if (answers === undefined) return;
    for (const each of this.#entries) {
      if (!answers.has(each.entry)) platform.workspaces.drop(each.entry);
    }
    this.#entries = platform.workspaces.list();
    this.#answers = answers;
  }

  /**
   * After a sign-in at a workspace's address: its entry, taken to where the list lives. Nothing
   * where there is no list, and nothing said where it fails - the next sign-in tries again.
   */
  async rememberHere(path: string): Promise<void> {
    const installation = this.installation;
    if (installation === undefined || this.isHome) return;
    const entry = await readOwnEntry();
    if (entry !== undefined) platform.workspaces.remember(entry, installation, path);
  }

  /**
   * At the installation's address: the change a redirect brought, carried out, and the browser sent
   * on. `false` where there was none, and the list is drawn.
   */
  async settle(): Promise<boolean> {
    const hop = platform.workspaces.takeHop();
    if (hop === undefined) return false;
    if (hop.kind === 'open') {
      // Only a workspace the list holds, and only while it answers: a link cannot send anybody
      // anywhere through this address (check 5).
      await this.show();
      const row = this.rows.find((each) => each.host === hop.host);
      if (row?.answers !== true) return false;
      platform.workspaces.leave(row.host, '/');
      return true;
    }
    const arriving = hop.kind === 'add' ? hop.entry : hop.back;
    if (hop.kind === 'forget') platform.workspaces.forget(hop.host, '', undefined, '/');
    const checked = (await this.#check([arriving]))?.get(arriving);
    // A forged entry, or a host that is no longer a main address, is not kept and opens nothing.
    if (checked?.answers !== true) return false;
    if (hop.kind === 'add') platform.workspaces.keep(arriving);
    platform.workspaces.leave(checked.host, hop.path);
    return true;
  }

  /** Opens a workspace of the list, which signs in on its own. */
  open(host: string): void {
    const installation = this.installation;
    if (installation === undefined) return;
    if (this.isHome) platform.workspaces.leave(host, '/');
    else platform.workspaces.open(host, installation);
  }

  /**
   * Forgets a workspace on this device: gone from the list at once, here and - by a redirect there
   * and back - where the list lives (check 4).
   */
  async forget(host: string): Promise<void> {
    const installation = this.installation;
    if (installation === undefined) return;
    if (this.isHome) {
      platform.workspaces.forget(host, installation, undefined, '/');
      this.#entries = platform.workspaces.list();
      return;
    }
    this.#entries = this.#entries.filter((each) => each.host !== host);
    const back = await readOwnEntry();
    platform.workspaces.forget(host, installation, back, location.pathname + location.search);
  }

  /** Which of these entries the installation signed, and whether each answers; nothing on failure. */
  async #check(entries: readonly string[]): Promise<Map<string, Checked> | undefined> {
    if (entries.length === 0) return new Map();
    try {
      const answer = await engine.mutate<DeviceListChecked>('POST', CHECK, { entries });
      const answers = new Map<string, Checked>();
      for (const each of answer.entries) answers.set(each.entry, { answers: each.answers, host: each.host });
      return answers;
    } catch {
      return undefined;
    }
  }
}

export const deviceList = new DeviceList();
