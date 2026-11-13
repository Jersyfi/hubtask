// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The release screen's choices (UC-PLN-08): which release it opens on, where a release's open
 * work may move, and which of a collection's entries may still join one.
 *
 * Pure, so that they are checked by `node --test`. The server decides what ships and writes the
 * notes; this module only picks from what it answered.
 */

import type { Release } from '@hubtask/sync-engine';

/** The planned releases a releasing one's open entries may move into: the soonest first. */
export function releaseTargets(releases: readonly Release[], releasing: string): Release[] {
  return releases
    .filter((r) => r.state === 'PLANNED' && r.id !== releasing)
    .sort((a, b) => a.release_on.localeCompare(b.release_on));
}

/** The release a screen opens on: the next planned one, else the latest released. */
export function releaseShown(releases: readonly Release[]): Release | undefined {
  return releaseTargets(releases, '')[0] ?? [...releases].sort((a, b) => b.release_on.localeCompare(a.release_on))[0];
}

/** The entries offered to join a release: those of the list not in it already. */
export function releaseCandidates<T extends { id: string }>(entries: readonly T[], inRelease: readonly string[]): T[] {
  const held = new Set(inRelease);
  return entries.filter((entry) => !held.has(entry.id));
}

/**
 * The releases as the screen lists them: the planned ones soonest first, those gone out latest
 * first, and the planned ones whose day has passed (`today` in the reader's zone) - said before
 * the day is long gone, not found later.
 */
export function releaseGroups(releases: readonly Release[], today: string): { planned: Release[]; released: Release[]; late: Set<string> } {
  const planned = releaseTargets(releases, '');
  const released = releases.filter((r) => r.state === 'RELEASED').sort((a, b) => b.release_on.localeCompare(a.release_on));
  return { planned, released, late: new Set(planned.filter((r) => r.release_on < today).map((r) => r.id)) };
}

/** A day as a number of days since the epoch, read in UTC: a release's day is a label, not a moment. */
const dayNumber = (day: string) => Math.round(Date.parse(`${day}T00:00:00Z`) / 86_400_000);
const monthStart = (day: string, shift = 0) => {
  const at = new Date(`${day.slice(0, 7)}-01T00:00:00Z`);
  at.setUTCMonth(at.getUTCMonth() + shift);
  return at.toISOString().slice(0, 10);
};

/**
 * The releases on an axis of months (scene planrel 2): from the month of the earliest of them
 * and today to the month after the latest, each release a mark at its day - the day it went out
 * where it did -, put on a second lane where it would cover its neighbour. Positions are fractions
 * of the axis, so the screen only scales them.
 */
export function releaseAxis(releases: readonly Release[], today: string, crowded = 0.12): {
  readonly months: readonly { readonly day: string; readonly at: number }[];
  readonly today: number;
  readonly marks: readonly { readonly id: string; readonly at: number; readonly lane: number }[];
} {
  const dayOf = (r: Release) => (r.state === 'RELEASED' && r.released_at ? r.released_at.slice(0, 10) : r.release_on);
  const days = [...releases.map(dayOf), today].sort();
  const from = monthStart(days[0] ?? today);
  const to = monthStart(days[days.length - 1] ?? today, 1);
  const span = Math.max(1, dayNumber(to) - dayNumber(from));
  const at = (day: string) => (dayNumber(day) - dayNumber(from)) / span;
  const months: { day: string; at: number }[] = [];
  for (let month = from; month < to; month = monthStart(month, 1)) months.push({ day: month, at: at(month) });
  const lanes: number[] = [];
  const marks = [...releases]
    .sort((a, b) => dayOf(a).localeCompare(dayOf(b)))
    .map((r) => {
      const where = at(dayOf(r));
      const lane = lanes.findIndex((last) => where - last >= crowded);
      const chosen = lane === -1 ? lanes.length : lane;
      lanes[chosen] = where;
      return { id: r.id, at: where, lane: chosen };
    });
  return { months, today: at(today), marks };
}
