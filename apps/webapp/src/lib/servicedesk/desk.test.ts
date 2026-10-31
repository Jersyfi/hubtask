// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { byPriorityOf, byPriorityText, formOf, holidaysOf, hoursOf, hubCalendarRows, reportRows } from './desk.ts';

test('a calendar is written from its days and one span, and read back the same (UC-SVC-01 check 1)', () => {
  const hours = hoursOf(['FRIDAY', 'MONDAY'], '08:00', '18:00');
  assert.deepEqual(hours, [{ day: 'MONDAY', from: '08:00', to: '18:00' }, { day: 'FRIDAY', from: '08:00', to: '18:00' }]);
  assert.deepEqual(formOf({ hours }), { days: ['MONDAY', 'FRIDAY'], from: '08:00', to: '18:00' });
  assert.deepEqual(holidaysOf('2026-12-25\n2026-12-26, 2027-01-01 '), ['2026-12-25', '2026-12-26', '2027-01-01']);
});

test('a goal’s targets per priority are read and written as pairs (UC-SVC-02 check 1)', () => {
  assert.deepEqual(byPriorityOf('P1=15, P2 = 60'), { P1: 15, P2: 60 });
  assert.deepEqual(byPriorityOf(''), {});
  assert.equal(byPriorityOf('P1=fast'), undefined);
  assert.equal(byPriorityOf('P1=0'), undefined);
  assert.equal(byPriorityText({ by_priority: { P1: 15, P2: 60 } }), 'P1=15, P2=60');
});

test('the report counts each policy’s goals by standing and the share met (UC-SVC-02 check 7)', () => {
  const rows = reportRows({
    data: [
      { policy_key: 'incidents', goal: 'RESOLUTION', standing: 'MET', entries: 3 },
      { policy_key: 'incidents', goal: 'RESOLUTION', standing: 'BREACHED', entries: 1 },
      { policy_key: 'incidents', goal: 'RESOLUTION', standing: 'RUNNING', entries: 2 },
      { policy_key: 'incidents', goal: 'FIRST_RESPONSE', standing: 'PAUSED', entries: 1 },
    ],
  });
  assert.deepEqual(rows[0], { policy: 'incidents', goal: 'RESOLUTION', met: 3, breached: 1, running: 2, paused: 0, metShare: 75 });
  assert.equal(rows[1]?.metShare, undefined);
});

test('a hub\'s own calendar stands beside the workspace\'s of its key (UC-SVC-01 check 5)', () => {
  const office = { key: 'office', name: 'Office', zone: 'Europe/Berlin', hours: hoursOf(['MONDAY'], '08:00', '18:00'), holidays: [], version: 1 };
  const night = { ...office, key: 'night', name: 'Night' };
  const ownOffice = { ...office, name: 'Office (Munich)', hub_id: 'h-1', holidays: ['2026-11-01'] };
  const rows = hubCalendarRows([office, night], [ownOffice]);
  assert.deepEqual(rows.map((row) => row.key), ['night', 'office']);
  assert.equal(rows[0]!.own, undefined);
  assert.equal(rows[1]!.own?.name, 'Office (Munich)');
});

test('a requester panel lists their other open requests, newest first, the request itself left out (UC-SVC-03 check 7)', async () => {
  const { otherRequests, requesterRequests, REQUESTER_OPEN } = await import('./desk.ts');
  const asked = requesterRequests('kim');
  assert.deepEqual(asked.filter.nodes[0], { op: 'EQ', field: 'created_by', value: 'kim' });
  assert.deepEqual(asked.filter.nodes[1], { op: 'EQ', field: 'is_completed', value: false });
  assert.equal(asked.page.size, REQUESTER_OPEN + 1);
  const rows = ['a', 'self', 'b', 'c', 'd', 'e', 'f'].map((id) => ({ id }));
  assert.deepEqual(otherRequests(rows, 'self').map((row) => row.id), ['a', 'b', 'c', 'd', 'e']);
});
