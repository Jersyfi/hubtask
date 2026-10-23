// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * One entry's kudos: the reader's own view of them, and the two writes (UC-WRK-29 checks 5, 6).
 *
 * **Online only.** Kudos are not a synchronised entity and the push frame carries no kudos, so the
 * engine refuses a write made offline (`sync.needs_connection`) rather than queueing it; the
 * control says so before it is pressed.
 *
 * **A write touches the kudos and nothing else.** Neither giving nor taking back moves the entry's
 * row, its version or its history, so no list is read again for it.
 */

import type { Kudos } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

/** Where an entry's kudos are read. One path, so two readers of the same entry share one read. */
export const kudosPath = (itemId: string) => `/items/${itemId}/kudos`;

const touches = (itemId: string) => [kudosPath(itemId)];

class KudosDesk {
  /** Gives the reader's kudos to whoever completed the entry, and answers the reader's view. */
  async give(itemId: string): Promise<Kudos> {
    return engine.mutate<Kudos>('PUT', kudosPath(itemId), undefined, { invalidates: touches(itemId) });
  }

  /** Takes the reader's kudos back, and answers the reader's view. */
  async takeBack(itemId: string): Promise<Kudos> {
    return engine.mutate<Kudos>('DELETE', kudosPath(itemId), undefined, { invalidates: touches(itemId) });
  }
}

export const kudos = new KudosDesk();
export type { Kudos };
