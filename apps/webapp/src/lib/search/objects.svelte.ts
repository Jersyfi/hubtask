// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The reads of the object sources (`objects.ts`) while a search or the palette asks: each list
 * once, kept as the engine answers it, matched by the caller as the words change.
 */

import { engine } from '../data/engine.ts';
import type { ObjectRow, ObjectSource } from './objects.ts';

export * from './objects.ts';

/** Subscribes to every read and hands each list's rows on; the returned function stops them. */
export function readObjects(
  reads: readonly { readonly path: string; readonly source: ObjectSource }[],
  onrows: (path: string, rows: readonly ObjectRow[]) => void,
): () => void {
  const stops = reads.map(({ path, source }) =>
    engine.subscribe<unknown>({ path }, (next) => {
      if (next.status === 'ready') onrows(path, source.rows(next.data));
    }),
  );
  return () => stops.forEach((stop) => stop());
}
