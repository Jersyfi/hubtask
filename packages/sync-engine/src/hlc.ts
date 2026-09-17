// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The hybrid logical clock, on this side of the wire (offline-sync.md §4.1).
//
// Device clocks are wrong, sometimes by hours. Every field change a device pushes therefore
// carries one of these rather than a bare timestamp: physical time, a counter that orders changes
// made within one millisecond, and the device that stamped it, which breaks the remaining ties the
// same way everywhere. The textual form is the server's - `core/domain/model/shared/HLC.go` -
// `<physical>:<counter>:<device>`, the first two zero-padded so that comparing two readings as
// strings compares them as clocks.
//
// What this module decides is `Tick`'s rule, and only that: the physical part never moves
// backwards. A wall clock that jumped back - a suspended laptop, a correction - would otherwise
// stamp a change that sorts before one already made, and the server would discard it on merge.
// When time has not moved on, the counter does; when the counter is exhausted, the clock borrows a
// millisecond from the future. The physical time comes from the `Clock` port and never from the
// machine directly, which is what lets a test fix it.

import type { Clock } from './ports.ts';

/** Milliseconds since the epoch, padded: enough digits until the year 2286. */
const PHYSICAL_DIGITS = 13;
/** How many changes one device can stamp inside one millisecond before the clock moves by itself. */
const COUNTER_DIGITS = 5;
export const HLC_MAX_COUNTER = 99_999;

export interface HlcReading {
  /** Milliseconds since the epoch, truncated to the millisecond. */
  readonly physical: number;
  readonly counter: number;
  readonly device: string;
}

/** The stored and transmitted form, as the server parses it. */
export function formatHlc({ physical, counter, device }: HlcReading): string {
  return `${String(physical).padStart(PHYSICAL_DIGITS, '0')}:${String(counter).padStart(COUNTER_DIGITS, '0')}:${device}`;
}

/**
 * Reads the textual form back, or nothing for a value the server would refuse too
 * (`sync.hlc_malformed`). A reading this client did not mint is still worth parsing: the server's
 * records carry one, and a device compares them to its own.
 */
export function parseHlc(raw: string): HlcReading | undefined {
  const first = raw.indexOf(':');
  if (first < 0) return undefined;
  const second = raw.indexOf(':', first + 1);
  if (second < 0) return undefined;
  const physical = Number(raw.slice(0, first));
  const counter = Number(raw.slice(first + 1, second));
  const device = raw.slice(second + 1);
  if (!Number.isInteger(physical) || physical < 0) return undefined;
  if (!Number.isInteger(counter) || counter < 0 || counter > HLC_MAX_COUNTER) return undefined;
  if (device === '') return undefined;
  return { physical, counter, device };
}

/** Physical time first, then the counter, then the device - the server's `Compare`. */
export function compareHlc(a: HlcReading, b: HlcReading): number {
  if (a.physical !== b.physical) return a.physical < b.physical ? -1 : 1;
  if (a.counter !== b.counter) return a.counter < b.counter ? -1 : 1;
  return a.device < b.device ? -1 : a.device > b.device ? 1 : 0;
}

/**
 * The next reading after `last`, at the clock's wall time: the server's `Tick`, with the same
 * three branches in the same order.
 */
export function tickHlc(last: HlcReading | undefined, now: number, device: string): HlcReading {
  const wall = Math.max(0, Math.trunc(now));
  if (last === undefined || wall > last.physical) return { physical: wall, counter: 0, device };
  if (last.counter >= HLC_MAX_COUNTER) return { physical: last.physical + 1, counter: 0, device };
  return { physical: last.physical, counter: last.counter + 1, device };
}

/**
 * One device's clock: the last reading it made, and the next one on demand. The queue stamps
 * every mutation from here; the reading is also what a device compares a server's record
 * against, which is why `observe` exists - a device that has seen a later reading must not stamp
 * an earlier one.
 */
export class HybridClock {
  readonly #clock: Clock;
  readonly #device: string;
  #last: HlcReading | undefined;

  constructor(clock: Clock, device: string, last?: HlcReading) {
    if (device === '' || device.includes(':')) {
      throw new TypeError('a device identifier has to be non-empty and free of colons - it is a segment of the clock');
    }
    this.#clock = clock;
    this.#device = device;
    this.#last = last;
  }

  /** The reading last handed out, for a caller that keeps it in the store across a reload. */
  get last(): HlcReading | undefined {
    return this.#last;
  }

  /** The next reading, and it is remembered. */
  next(): HlcReading {
    this.#last = tickHlc(this.#last, this.#clock.now(), this.#device);
    return this.#last;
  }

  /** The next reading, formatted - what a mutation carries. */
  stamp(): string {
    return formatHlc(this.next());
  }

  /**
   * A reading seen from elsewhere - a record the server sent. The clock moves past it, so that
   * the next stamp sorts after everything this device has seen, whatever its wall clock says.
   */
  observe(reading: HlcReading): void {
    if (this.#last === undefined || compareHlc(reading, this.#last) > 0) {
      this.#last = { physical: reading.physical, counter: reading.counter, device: this.#device };
    }
  }
}
