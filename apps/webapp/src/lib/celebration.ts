// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which moment a completion is (design-system.md §7, F6-13).
 *
 * Decided from the hierarchy the client already holds - the completion it just performed and the
 * entries and containers in the replica - and from nothing else. §7's table, as a function:
 *
 * - every completion is **1**;
 * - the last open entry under its parent is **2**: the last activity closes a work package, the
 *   last work package a task, and the parent is what the completed entry sits under;
 * - a collection or a hub with nothing open after it, the last entry due today across what the
 *   copy holds, or a completion while the account's tour has not ended - the first-ever
 *   completion as the onboarding moment (§8) - is **3**, at most once a day; a second qualifying
 *   moment the same day falls back to 2.
 *
 * Nothing heuristic and nothing random. The cap is the caller's to keep (`lastTier3On`), because
 * this function holds no state: the same hierarchy always answers the same tier.
 */

/** What the tier reads of an entry: its place in the tree and whether it is done. */
export interface HeldEntry {
  readonly id: string;
  readonly parent_id?: string | null;
  readonly collection_id?: string | null;
  readonly is_completed: boolean;
  /** The due moment as the API spells it, or nothing. */
  readonly due_at?: string | null;
  /** Trashed or archived entries are not open and not counted. */
  readonly deleted_at?: string | null;
  readonly archived_at?: string | null;
}

/** What the tier reads of a container: which hub a collection sits under. */
export interface HeldContainer {
  readonly id: string;
  readonly type: string;
  readonly parent_id?: string | null;
}

export interface CelebrationContext {
  /** The entry the person just completed, as the copy holds it now. */
  readonly completedId: string;
  readonly entries: readonly HeldEntry[];
  readonly containers: readonly HeldContainer[];
  /** Today where the person is, `YYYY-MM-DD`, and the zone that decided it. */
  readonly today: string;
  readonly zone: string;
  /** The day the last tier-3 moment was shown, `YYYY-MM-DD`, or nothing. */
  readonly lastTier3On?: string;
  /** Whether the account's tour has not ended: `onboarding_completed_at` is null. */
  readonly isBeforeOnboarding: boolean;
}

export type Tier = 1 | 2 | 3;

/** Why the tier is what it is - what the announcement says (voice-and-tone.md §7). */
export type Reason = 'completion' | 'parent' | 'collection' | 'hub' | 'day' | 'first';

export interface Moment {
  readonly tier: Tier;
  readonly reason: Reason;
  /** True when the moment would have been 3 and the day's one was already spent. */
  readonly isCapped: boolean;
}

const isLive = (entry: HeldEntry) => !entry.deleted_at && !entry.archived_at;
const isOpen = (entry: HeldEntry) => isLive(entry) && !entry.is_completed;

/** The calendar day of an instant where the person is, or nothing for an entry without one. */
export function dayOf(dueAt: string | null | undefined, zone: string): string | undefined {
  if (!dueAt) return undefined;
  const at = new Date(dueAt);
  if (Number.isNaN(at.getTime())) return undefined;
  try {
    // `en-CA` writes YYYY-MM-DD, which is the one thing asked of it here.
    return new Intl.DateTimeFormat('en-CA', { timeZone: zone, year: 'numeric', month: '2-digit', day: '2-digit' }).format(at);
  } catch {
    return at.toISOString().slice(0, 10);
  }
}

/**
 * The moment of a completion. Answers `parent`, `collection`, `hub`, `day` and `first` by the
 * first rule that applies, in the order of §7's table read from the top of the tree: the rarest
 * moment wins the sentence, and the cap decides the tier afterwards.
 */
export function momentOf(context: CelebrationContext): Moment {
  const { entries, containers, completedId } = context;
  const completed = entries.find((entry) => entry.id === completedId);
  if (!completed || !completed.is_completed) return { tier: 1, reason: 'completion', isCapped: false };

  const openUnder = (predicate: (entry: HeldEntry) => boolean) => entries.some((entry) => isOpen(entry) && predicate(entry));

  const spent = context.lastTier3On === context.today;
  let big: Reason | undefined;
  if (context.isBeforeOnboarding && !spent) {
    // The onboarding moment (§8): the first completion of the day before the tour ended. Once
    // the day's tier 3 is spent this claims nothing - "that was your first" is not a sentence
    // for the second - and the completion is read like any other.
    big = 'first';
  } else if (completed.collection_id) {
    const collection = containers.find((each) => each.id === completed.collection_id);
    const hubId = collection?.parent_id ?? undefined;
    const collectionIds = hubId
      ? new Set(containers.filter((each) => each.parent_id === hubId).map((each) => each.id))
      : new Set([completed.collection_id]);
    if (hubId && !openUnder((entry) => !!entry.collection_id && collectionIds.has(entry.collection_id))) {
      big = 'hub';
    } else if (!openUnder((entry) => entry.collection_id === completed.collection_id)) {
      big = 'collection';
    }
  }
  if (!big) {
    // The day's close: the completed entry was due today, and nothing else due today is open.
    const dueToday = (entry: HeldEntry) => dayOf(entry.due_at, context.zone) === context.today;
    if (dueToday(completed) && !openUnder(dueToday)) big = 'day';
  }
  if (big) {
    return spent ? { tier: 2, reason: big, isCapped: true } : { tier: 3, reason: big, isCapped: false };
  }

  if (completed.parent_id && !openUnder((entry) => entry.parent_id === completed.parent_id)) {
    return { tier: 2, reason: 'parent', isCapped: false };
  }
  return { tier: 1, reason: 'completion', isCapped: false };
}
