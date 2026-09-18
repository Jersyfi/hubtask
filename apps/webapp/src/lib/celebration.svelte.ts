// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The moment a completion earns, held for the screen that made it (F6-13).
 *
 * One current celebration at a time and never a queue: §7 says a celebration is the exception,
 * and a second completion during the first replaces it rather than waiting behind it - nothing is
 * delayed. The tier is `lib/celebration.ts`'s, read from the replica the engine holds; the daily
 * cap is a record in the store's own metadata, so that it survives a reload and is the account's
 * copy's rather than the tab's. Without a store attached - online only - the copy cannot be read,
 * and every completion is tier 1: a screen that cannot see the hierarchy claims nothing about it.
 *
 * **Off means nothing is mounted.** The switch is the account's `celebrations` preference (F6-12);
 * a screen asks `isOn` before rendering the component, and this module answers no moment while it
 * is off.
 */

import type { WorkItem } from '@hubtask/sync-engine';
import { META } from '@hubtask/sync-engine';

import { momentOf, type HeldContainer, type HeldEntry, type Moment } from './celebration.ts';
import { actor } from './data/account.svelte.ts';
import { engine } from './data/engine.ts';
import { todayIn } from './i18n/zone.ts';
import { tour } from './tour.svelte.ts';

/** The store's own record of the last tier-3 moment: one per copy, under `meta`. */
const CAP_KEY = 'celebration';

interface Cap {
  readonly lastTier3On: string;
}

export interface Current extends Moment {
  /** The completed entry. What the announcement names. */
  readonly item: WorkItem;
  /** Which screen made it: only that one mounts the slot. */
  readonly token: number;
}

type Document = Record<string, unknown>;

class Celebration {
  #current = $state<Current | undefined>(undefined);
  #next = 0;

  /** The account's switch (F6-12): absent means on, §7's default. */
  get isOn(): boolean {
    return actor.account?.celebrations !== false;
  }

  get current(): Current | undefined {
    return this.isOn ? this.#current : undefined;
  }

  /**
   * A completion the person just performed, as the server answered it. Decides the tier from the
   * copy and shows the moment; answers what it decided, or nothing while the switch is off.
   */
  async celebrate(item: WorkItem): Promise<Current | undefined> {
    if (!item.completion?.is_completed) return undefined;
    // The tour's end and the first moment coincide (§8, F6-14): the first completion of an
    // account that has not taken the tour writes `onboarding_completed_at` - whether or not the
    // moments are marked, because the tour ended either way.
    if (!actor.account?.onboarding_completed_at) void tour.complete();
    if (!this.isOn) return undefined;
    const token = ++this.#next;
    const moment = await this.#momentOf(item);
    if (token !== this.#next) return undefined;
    if (moment.tier === 3) await this.#spend();
    const current: Current = { ...moment, item, token };
    this.#current = current;
    return current;
  }

  /** The slot ended, or the screen left: nothing is shown any more. */
  dismiss(token?: number): void {
    if (token !== undefined && this.#current?.token !== token) return;
    this.#current = undefined;
  }

  async #momentOf(item: WorkItem): Promise<Moment> {
    const storage = engine.storage;
    const zone = actor.zone;
    const today = todayIn(zone);
    const isBeforeOnboarding = !actor.account?.onboarding_completed_at;
    if (!storage) {
      // Online only: no copy to read the hierarchy from. The first-ever moment is still knowable.
      return momentOf({ completedId: item.id, entries: [held(item)], containers: [], today, zone, isBeforeOnboarding });
    }
    const [items, containers, cap] = await Promise.all([
      storage.all<{ document: Document }>('items'),
      storage.all<{ document: Document }>('containers'),
      storage.get<Cap>(META, CAP_KEY),
    ]);
    const entries = items.map((record) => held(record.document as unknown as WorkItem));
    // The copy may not have the server's answer yet: the completion just made is laid over it.
    const known = entries.some((entry) => entry.id === item.id);
    return momentOf({
      completedId: item.id,
      entries: known ? entries.map((entry) => (entry.id === item.id ? held(item) : entry)) : [...entries, held(item)],
      containers: containers.map((record) => record.document as unknown as HeldContainer),
      today,
      zone,
      lastTier3On: cap?.lastTier3On,
      isBeforeOnboarding,
    });
  }

  /** The day's tier 3 is spent: written to the store's metadata, where the next reload finds it. */
  async #spend(): Promise<void> {
    const storage = engine.storage;
    if (!storage) return;
    await storage.put<Cap>(META, CAP_KEY, { lastTier3On: todayIn(actor.zone) });
  }
}

function held(item: WorkItem): HeldEntry {
  return {
    id: item.id,
    parent_id: item.parent_id ?? null,
    collection_id: item.collection_id ?? null,
    is_completed: item.completion?.is_completed === true,
    due_at: item.due_at ?? null,
    deleted_at: item.deleted_at ?? null,
    archived_at: item.archived_at ?? null,
  };
}

export const celebration = new Celebration();
