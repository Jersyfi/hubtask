// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The keys of the insert list (`InsertList`): the menus' roving and type-ahead, with Enter taking the
// active choice and Escape closing. A function over data, so the contract is tested without a browser.

import { rovingIndex, typeAheadIndex } from './focus.ts';

/** The new active index, `take`, `close`, or null when the key is not the list's. */
export function listKey(key: string, active: number, labels: readonly string[]): number | 'take' | 'close' | null {
  if (key === 'Enter') return labels.length > 0 ? 'take' : null;
  if (key === 'Escape') return 'close';
  return rovingIndex(key, active, labels.length) ?? typeAheadIndex(key, active, labels);
}
