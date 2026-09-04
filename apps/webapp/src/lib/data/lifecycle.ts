// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The two lifecycle states that are not deletion, and the one that is.
 *
 * `domain-model.md` §3.4's state machine is the specification: active → archived and back, active
 * or archived → trashed, trashed → active within the retention period, and then the retention job.
 * The distinctions this module makes are the ones a screen gets wrong.
 *
 * **Archived is read-only, not hidden** (I-C3, I-W4). An archived thing is reachable, readable,
 * and every control on it is off *with the reason* — which is what `CapabilityGate` was built for
 * and what a screen that removed the buttons would fail to say.
 *
 * **Archived in its own right is not the same as archived from above.** A container separates
 * `archived_at` from `effective_archived` precisely so a client can tell them apart, and they are
 * different offers: the first has an unarchive control here, the second has one on the thing above
 * it and none here. An **entry** has only `archived_at` — its inheritance is not a field but the
 * position it is in, so the caller says whether the thing above it is archived.
 */

import type { WorkItem } from '@hubtask/sync-engine';

import type { Archival } from './containers.ts';

/**
 * Where an entry stands in the archive.
 *
 * `isBelowArchived` is the caller's, because the contract does not give an entry an
 * `effective_archived`: an archived collection makes its entries read-only (I-C3) and an archived
 * entry makes its children read-only, and both of those are things the screen already knows from
 * what it is drawing.
 */
export function archivalOfItem(item: WorkItem, isBelowArchived: boolean): Archival {
  if (item.archived_at) return 'archived';
  return isBelowArchived ? 'inherited' : 'active';
}

/** Milliseconds in a day, named once so the arithmetic below reads as what it is. */
const DAY = 24 * 60 * 60 * 1000;

/**
 * How many whole days are left before the retention job takes this for good.
 *
 * Rounded **up**, because a thing with eleven hours left has a day left as far as a decision goes,
 * and rounding down would tell somebody they had none while restore still worked. Never below
 * zero: a row past its period is one the job has not reached yet, and "−2 days" is not a sentence.
 *
 * `undefined` when the window is not known, which is the ordinary case rather than an error — the
 * period is the workspace's configuration and reading it needs `retention:read`, which an ordinary
 * member does not have. A number invented here would be a promise this client cannot keep.
 */
export function remainingDays(
  deletedAt: string,
  retainDays: number | undefined,
  now: number,
): number | undefined {
  if (retainDays === undefined) return undefined;
  const deleted = Date.parse(deletedAt);
  if (Number.isNaN(deleted)) return undefined;
  return Math.max(0, Math.ceil((deleted + retainDays * DAY - now) / DAY));
}

/**
 * Which endpoint puts a row back, and which one destroys it.
 *
 * The trash mixes containers and entries by design — the contract says so — so every row carries
 * the `kind` that says which of the two it is. A screen that guessed from the subtype would be
 * reading `HUB` and `TASK` out of a field the contract documents as free text.
 */
export function isContainer(kind: string): boolean {
  return kind === 'CONTAINER';
}
