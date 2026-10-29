// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import test from 'node:test';

import type { OwnDomain } from '@hubtask/sync-engine';

import { canBecomeMain, fallBackOf, isRemovable, recordsShown, toneOf, zoneOf } from './owndomain.ts';

const records: OwnDomain['records'] = [
  { type: 'CNAME', name: 'tasks.acme.com', value: 'domains.tasks.example.net', found: false },
  { type: 'TXT', name: '_hubtask.tasks.acme.com', value: 'hubtask-domain=q7n4kx2p8zram3vd', found: false },
];

function own(state: OwnDomain['state']): OwnDomain {
  return { domain: 'tasks.acme.com', state, since: '2026-10-27T09:10:00Z', is_main: false, certificate_asked: false, records };
}

test('the records are added in the zone the domain lies in', () => {
  assert.equal(zoneOf('tasks.acme.com'), 'acme.com');
  assert.equal(zoneOf('board.tasks.acme.co.uk'), 'tasks.acme.co.uk');
});

test('UC-INS-15 check 2: waiting shows both records; broken the proof alone; the others none', () => {
  assert.deepEqual(recordsShown(own('PENDING')).map((r) => r.type), ['CNAME', 'TXT']);
  assert.deepEqual(recordsShown(own('BROKEN')).map((r) => r.type), ['TXT']);
  assert.equal(recordsShown(own('VERIFIED')).length, 0);
  assert.equal(recordsShown(own('ACTIVE')).length, 0);
});

test('UC-INS-15 check 3: each state has its tone, and removal where the screens offer it', () => {
  assert.equal(toneOf('PENDING'), 'info');
  assert.equal(toneOf('VERIFIED'), 'success');
  assert.equal(toneOf('ACTIVE'), 'success');
  assert.equal(toneOf('BROKEN'), 'danger');
  assert.equal(toneOf('BLOCKED'), 'danger');
  assert.deepEqual(
    (['PENDING', 'VERIFIED', 'ACTIVE', 'BROKEN', 'BLOCKED'] as const).filter(isRemovable),
    ['PENDING', 'ACTIVE', 'BLOCKED'],
  );
});

test('UC-INS-15 check 6: only an active domain that is not main yet is offered as the main address', () => {
  assert.equal(canBecomeMain(own('ACTIVE')), true);
  assert.equal(canBecomeMain({ ...own('ACTIVE'), is_main: true }), false);
  for (const state of ['PENDING', 'VERIFIED', 'BROKEN', 'BLOCKED'] as const) assert.equal(canBecomeMain(own(state)), false, state);
});

test('UC-INS-15 check 9: the warning after a fall-back, broken or blocked, and none otherwise', () => {
  assert.deepEqual(fallBackOf({ ...own('BROKEN'), fell_back_at: '2026-10-27T09:10:00Z' }), { cause: 'broken', at: '2026-10-27T09:10:00Z' });
  // It stays once the record is back, until the domain is made main again.
  assert.equal(fallBackOf({ ...own('ACTIVE'), fell_back_at: '2026-10-27T09:10:00Z' })?.cause, 'broken');
  assert.deepEqual(fallBackOf({ ...own('BLOCKED'), is_main: true }), { cause: 'blocked', at: '2026-10-27T09:10:00Z' });
  assert.equal(fallBackOf(own('BLOCKED')), undefined);
  assert.equal(fallBackOf(own('BROKEN')), undefined);
  assert.equal(fallBackOf(undefined), undefined);
});
