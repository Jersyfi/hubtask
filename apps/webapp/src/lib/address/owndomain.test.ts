// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import test from 'node:test';

import type { OwnDomain } from '@hubtask/sync-engine';

import { isRemovable, recordsShown, toneOf, zoneOf } from './owndomain.ts';

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
