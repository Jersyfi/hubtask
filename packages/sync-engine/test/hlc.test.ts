// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The hybrid logical clock and the device identity, over a clock the test moves (rule 4).

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { HybridClock, HLC_MAX_COUNTER, compareHlc, formatHlc, parseHlc, tickHlc } from '../src/hlc.ts';
import { isUuidV7, mintUuidV7 } from '../src/device.ts';
import { FixedClock } from './fakes.ts';

test('a reading is written in the textual form the server parses, and reads back', () => {
  const reading = { physical: 1_700_000_000_000, counter: 7, device: 'dev-a3' };
  const text = formatHlc(reading);
  assert.equal(text, '1700000000000:00007:dev-a3');
  assert.deepEqual(parseHlc(text), reading);
  // Zero-padded, so the strings compare as the clocks do.
  assert.ok(formatHlc({ ...reading, physical: 999 }) < text);
  assert.ok(formatHlc({ ...reading, counter: 8 }) > text);
});

test('what the server would refuse as malformed is not a reading', () => {
  for (const raw of ['', '1700000000000', '1700000000000:7', 'x:0:d', '1700000000000:00007:', '1700000000000:100000:d', '-1:0:d']) {
    assert.equal(parseHlc(raw), undefined, raw);
  }
});

test('the physical part never moves backwards, the counter orders within a millisecond', () => {
  const clock = new FixedClock(1_700_000_000_000);
  const hlc = new HybridClock(clock, 'dev-a');

  const first = hlc.next();
  assert.deepEqual(first, { physical: 1_700_000_000_000, counter: 0, device: 'dev-a' });
  assert.deepEqual(hlc.next(), { physical: 1_700_000_000_000, counter: 1, device: 'dev-a' }, 'time stood still: the counter moves');

  clock.advance(-5_000);
  const afterAJumpBack = hlc.next();
  assert.equal(afterAJumpBack.physical, 1_700_000_000_000, 'a wall clock that jumped back is ignored');
  assert.equal(afterAJumpBack.counter, 2);

  clock.advance(6_000);
  assert.deepEqual(hlc.next(), { physical: 1_700_000_001_000, counter: 0, device: 'dev-a' }, 'time moved on: the counter resets');
});

test('an exhausted counter borrows a millisecond from the future', () => {
  const at = { physical: 1_700_000_000_000, counter: HLC_MAX_COUNTER, device: 'd' };
  assert.deepEqual(tickHlc(at, 1_700_000_000_000, 'd'), { physical: 1_700_000_000_001, counter: 0, device: 'd' });
});

test('a reading seen from elsewhere moves the clock past it', () => {
  const clock = new FixedClock(1_700_000_000_000);
  const hlc = new HybridClock(clock, 'dev-a');
  hlc.observe({ physical: 1_700_000_009_000, counter: 3, device: 'dev-b' });
  const next = hlc.next();
  assert.ok(compareHlc(next, { physical: 1_700_000_009_000, counter: 3, device: 'dev-b' }) > 0, 'the next stamp sorts after what was seen');
  assert.equal(next.device, 'dev-a', 'and it is still this device\'s');
});

test('the order is physical, then counter, then device - the same everywhere', () => {
  const a = { physical: 1, counter: 0, device: 'a' };
  assert.ok(compareHlc(a, { ...a, physical: 2 }) < 0);
  assert.ok(compareHlc(a, { ...a, counter: 1 }) < 0);
  assert.ok(compareHlc(a, { ...a, device: 'b' }) < 0);
  assert.equal(compareHlc(a, { ...a }), 0);
});

test('a device identifier is a UUIDv7 at the clock\'s time, and a colon in one is refused', () => {
  const clock = new FixedClock(1_700_000_000_000);
  const id = mintUuidV7(clock);
  assert.ok(isUuidV7(id), id);
  // The first 48 bits are the milliseconds, so the identifier sorts by the moment it was minted.
  assert.equal(Number.parseInt(id.replaceAll('-', '').slice(0, 12), 16), 1_700_000_000_000);
  assert.notEqual(mintUuidV7(clock), id, 'the rest is random');
  assert.throws(() => new HybridClock(clock, 'a:b'), TypeError);
});
