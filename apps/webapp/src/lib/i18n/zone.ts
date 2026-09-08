// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Wall-clock time in a named zone, and the instant behind it.
 *
 * The contract stores a due date as three fields written together — the instant, the all-day flag
 * and the IANA zone — because the three describe one date (`i18n-l10n.md` §4). A client that read
 * only the instant would draw a date that moves: an all-day date set in Berlin would show as the
 * day before to a reader in São Paulo, which is the exact defect the flag and the zone exist to
 * prevent.
 *
 * So the conversion happens **in the entry's zone, not the browser's**, in both directions, and it
 * happens here rather than in a component. `Intl` knows every zone the platform knows; the arithmetic
 * around it is two functions, and a dependency for two functions is a supply-chain decision nobody
 * needs to take.
 *
 * **No `Date` maths on local time anywhere.** `new Date(y, m, d)` builds a moment in the machine's
 * zone, and the machine's zone is the one thing that must not decide what a date means here.
 */

/** The parts of a wall clock, as numbers. */
interface Wall {
  readonly year: number;
  readonly month: number;
  readonly day: number;
  readonly hour: number;
  readonly minute: number;
}

const pad = (value: number, width = 2) => String(value).padStart(width, '0');

/** What the clock in `zone` reads at that instant. */
function wallIn(instant: number, zone: string): Wall {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: zone,
    hour12: false,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).formatToParts(new Date(instant));

  const of = (type: string) => Number(parts.find((part) => part.type === type)?.value ?? '0');
  // `hour12: false` answers 24 for midnight in some engines, which is the same moment written
  // differently. Normalising it here keeps every caller from having to know that.
  const hour = of('hour') % 24;
  return { year: of('year'), month: of('month'), day: of('day'), hour, minute: of('minute') };
}

/** The same wall clock, read as if it were UTC. The intermediate every offset calculation needs. */
const asUTC = (wall: Wall) =>
  Date.UTC(wall.year, wall.month - 1, wall.day, wall.hour, wall.minute);

/** How far `zone` is from UTC at that instant, in milliseconds. */
function offsetAt(instant: number, zone: string): number {
  return asUTC(wallIn(instant, zone)) - instant;
}

/**
 * The instant at which the clock in `zone` reads this date and time.
 *
 * Two passes, and the second is not belt and braces: the offset depends on the instant, and the
 * instant is what is being solved for. One guess lands within an hour of the answer everywhere on
 * earth; the second pass uses the offset *at that guess*, which is the right one on every day
 * except the two an hour long. Those two are why this is not a subtraction.
 *
 * A time that does not exist — 02:30 on the morning a zone springs forward — resolves to the
 * instant the clock jumps to, which is what every calendar does with it.
 */
export function instantOf(date: string, time: string | undefined, zone: string): string | undefined {
  const wall = Date.parse(`${date}T${time ?? '00:00'}:00Z`);
  if (Number.isNaN(wall)) return undefined;

  let guess = wall - offsetAt(wall, zone);
  guess = wall - offsetAt(guess, zone);
  return new Date(guess).toISOString();
}

/** The date the clock in `zone` shows at that instant, as `YYYY-MM-DD`. */
export function dateIn(iso: string, zone: string): string | undefined {
  const instant = Date.parse(iso);
  if (Number.isNaN(instant)) return undefined;
  const wall = wallIn(instant, zone);
  return `${pad(wall.year, 4)}-${pad(wall.month)}-${pad(wall.day)}`;
}

/** The time the clock in `zone` shows at that instant, as `HH:mm`. */
export function timeIn(iso: string, zone: string): string | undefined {
  const instant = Date.parse(iso);
  if (Number.isNaN(instant)) return undefined;
  const wall = wallIn(instant, zone);
  return `${pad(wall.hour)}:${pad(wall.minute)}`;
}

/** Today, where the reader is. Never `new Date().toISOString()`, which is today in Greenwich. */
export function todayIn(zone: string, now: number = Date.now()): string {
  const wall = wallIn(now, zone);
  return `${pad(wall.year, 4)}-${pad(wall.month)}-${pad(wall.day)}`;
}

/** The zone the platform is set to. The fallback when an account states none. */
export function deviceZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
}

/** The zone's short name as the reader's locale writes it — "GMT+2", "BRT", "Europe/Berlin". */
export function zoneName(iso: string, zone: string, locale: string): string {
  const instant = Date.parse(iso);
  if (Number.isNaN(instant)) return zone;
  const parts = new Intl.DateTimeFormat(locale, { timeZone: zone, timeZoneName: 'short' })
    .formatToParts(new Date(instant));
  return parts.find((part) => part.type === 'timeZoneName')?.value ?? zone;
}
