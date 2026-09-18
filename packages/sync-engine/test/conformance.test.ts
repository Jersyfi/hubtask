// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The conformance runner (F6-08) held to its own claims: against a server in memory that keeps
// offline-sync.md's promises it passes every testable point; against a deliberately wrong engine
// - one whose store keeps what an ACCESS_REVOKED record tells it to drop - it fails point 3 and
// only point 3. A runner that cannot go red proves nothing.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { MemoryStorage } from '../src/storage/MemoryStorage.ts';
import type { Storage } from '../src/ports.ts';
import { runConformance } from '../conformance/run.ts';
import { FakeServer } from './fakeServer.ts';

/** A clock that moves: the runner's deadlines and the UUIDv7 mint both need it to. */
const movingClock = { now: () => Date.now() };

const outcomes = (rows: readonly { number: number; outcome: string }[]) =>
  Object.fromEntries(rows.map((row) => [row.number, row.outcome]));

test('against a server that keeps the protocol, every testable point passes and the fixtures are gone', async () => {
  const server = new FakeServer(movingClock);
  const lines: string[] = [];
  const run = await runConformance({
    baseUrl: 'fake://server',
    token: server.ownerToken,
    transportFor: (token) => server.transportFor(token),
    clock: movingClock,
    log: (line) => lines.push(line),
    deadlineMs: 5_000,
  });

  assert.equal(run.failed, 0, run.rows.filter((r) => r.outcome === 'fail').map((r) => `${r.number}: ${r.note}`).join('\n'));
  assert.deepEqual(outcomes(run.rows), {
    1: 'pass', 2: 'pass', 3: 'pass', 4: 'pass', 5: 'pass', 6: 'not-tested', 7: 'pass', 8: 'not-tested',
  });
  assert.equal(run.rows.map((r) => r.number).join(''), '12345678', 'one row per requirement, in order');
  assert.match(run.rows[5]?.note ?? '', /ADR-0033 §4/);
  assert.match(run.rows[7]?.note ?? '', /by construction/);
  assert.match(run.report, /^# Sync conformance — offline-sync\.md §9, the engine/);
  assert.match(run.report, /\| Checks \| 8, 0 failed, 2 not testable through the engine \|/);
  assert.ok(lines.some((line) => /^ {2}1\. pass/.test(line)), 'each point prints its number and its result');

  // Tidied away: the hub, the token, the membership.
  assert.equal(server.containers.size, 0);
  assert.equal(server.tokens.size, 0);
  assert.equal(server.memberships.size, 0);
  // Every push carried the device frame.
  for (const push of server.pushes) {
    const body = push.body as { device_id: string; platform: string };
    assert.match(body.device_id, /^[0-9a-f-]{36}$/);
    assert.equal(body.platform, 'node');
  }
});

/** A store that keeps what it is told to drop below a hub: the wrong engine of the acceptance. */
class KeepingStorage extends MemoryStorage implements Storage {
  override async delete(collection: string, id: string): Promise<void> {
    if (collection === 'containers' || collection === 'items') return;
    return super.delete(collection, id);
  }
}

test('a deliberately wrong engine - the store keeps what ACCESS_REVOKED tells it to drop - fails exactly point 3', async () => {
  const server = new FakeServer(movingClock);
  const run = await runConformance({
    baseUrl: 'fake://server',
    token: server.ownerToken,
    transportFor: (token) => server.transportFor(token),
    clock: movingClock,
    storage: () => new KeepingStorage(),
    deadlineMs: 5_000,
  });

  assert.equal(run.failed, 1);
  assert.deepEqual(outcomes(run.rows), {
    1: 'pass', 2: 'pass', 3: 'fail', 4: 'pass', 5: 'pass', 6: 'not-tested', 7: 'pass', 8: 'not-tested',
  });
  assert.match(run.rows[2]?.note ?? '', /still holds 2 container\(s\)/);
});
