// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Keeping a collection's figure true without reloading the navigation on every keystroke.
 *
 * The server counts a figure on the collection's read (domain-model.md §2), so what makes it stale
 * is a change to the collection's entries, or to the number field it sums, and the reader's day
 * turning over. Neither is a container record, so `live.ts` names no container for them; this
 * module is told of each record and **coalesces**: changes arriving within `SETTLE_MS` re-read each
 * collection's own document and each hub's level once, rather than once per record - ticking off
 * ten groceries is one re-read, not ten.
 *
 * Once a minute it compares each figure's `today` with the reader's date in their own zone; a
 * figure counted for yesterday is read again, which is also how the week turns over (`RHYTHM`).
 */

import type { ChangeRecord } from '@hubtask/sync-engine';

import { todayIn } from '../i18n/zone.ts';
import { actor } from './account.svelte.ts';
import { containers } from './containers.svelte.ts';
import { figureTargets, isStale } from './figure.ts';

/** How long changes are gathered before the figures they touch are read again. */
const SETTLE_MS = 1000;

/** How often the reader's date is compared with the figures'. */
const DAY_CHECK_MS = 60_000;

/** The records that can move a figure: an entry, or a field definition a sum reads. */
const ENTRY_ENTITIES = new Set(['item', 'work_item']);
const FIELD_ENTITIES = new Set(['custom_field', 'custom_field_definition']);

class Figures {
  #pending = new Set<string>();
  #timer: ReturnType<typeof setTimeout> | undefined;

  /** Told of every change record; gathers the collections whose figure it may have moved. */
  note(record: ChangeRecord): void {
    const collection = record.container_id ?? undefined;
    if (ENTRY_ENTITIES.has(record.entity)) {
      if (collection) this.#gather([collection]);
      return;
    }
    if (FIELD_ENTITIES.has(record.entity)) {
      // A workspace-wide field reaches every collection that sums it.
      this.#gather(
        collection
          ? [collection]
          : containers.loadedCollections.filter((c) => c.figure?.kind === 'FIELD_SUM').map((c) => c.id),
      );
    }
  }

  /** Starts the daily comparison; the frame calls it once a session exists. */
  start(): () => void {
    const check = () => {
      const today = todayIn(actor.zone);
      this.#gather(containers.loadedCollections.filter((c) => isStale(c.figure, today)).map((c) => c.id));
    };
    const interval = setInterval(check, DAY_CHECK_MS);
    return () => {
      clearInterval(interval);
      if (this.#timer !== undefined) clearTimeout(this.#timer);
      this.#timer = undefined;
      this.#pending.clear();
    };
  }

  #gather(ids: readonly string[]): void {
    if (ids.length === 0) return;
    for (const id of ids) this.#pending.add(id);
    this.#timer ??= setTimeout(() => this.#flush(), SETTLE_MS);
  }

  #flush(): void {
    this.#timer = undefined;
    const ids = [...this.#pending];
    this.#pending.clear();
    void containers.refreshFigures(figureTargets(ids, (id) => containers.find(id)?.parent_id));
  }
}

export const figures = new Figures();
