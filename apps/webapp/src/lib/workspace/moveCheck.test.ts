// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { format } from '../i18n/format.ts';
import { SOURCE } from '../i18n/source.ts';
import type { MoveFinding } from '@hubtask/sync-engine';

import { findingWords, isObstacleResolved } from './moveCheck.ts';

const t = (code: string, params: Record<string, string | number> = {}) => format(SOURCE[code] ?? code, params);
const context = { from: 'Product development', to: 'IT operations', nameOf: (id: string) => ({ a1: 'Mara Lind', a2: 'Tim Berg' })[id] ?? '', hubName: () => '' };

test('UC-WRK-39 check 7: every finding is said in words, never as its code', () => {
  const findings: MoveFinding[] = [
    { code: 'containers.name_taken', params: {} },
    { code: 'entry_types.hub_bound', params: {} },
    { code: 'workflows.hub_bound', params: {} },
    { code: 'access.not_permitted', params: { permission: 'STRUCTURE', hub_id: 'h' } },
    { code: 'containers.move_loses_access', params: { count: '2', account_ids: 'a1,a2' } },
    { code: 'containers.move_takes_structure', params: { hub_id: 'h' } },
    { code: 'containers.move_keeps_releases', params: { count: '4', hub_id: 'h' } },
    { code: 'containers.move_keeps_sla_policies', params: { count: '2', hub_id: 'h' } },
    { code: 'something.new', params: {} },
  ];
  for (const finding of findings) {
    const said = findingWords(finding, context, t);
    assert.ok(!said.includes('.') || said.endsWith('.'), said);
    assert.ok(!/[a-z]+_[a-z]+/.test(said) && !said.startsWith('app.'), `${finding.code} is not worded: ${said}`);
  }
  assert.match(findingWords(findings[4]!, context, t), /2 people .*Mara Lind, Tim Berg/);
  assert.match(findingWords(findings[0]!, context, t), /IT operations/);
  assert.match(findingWords(findings[6]!, context, t), /4 entries .*Product development/);
});

test('UC-WRK-39 check 8: a name taken is resolved by a new name, nothing else in the dialog', () => {
  const taken: MoveFinding = { code: 'containers.name_taken', params: {} };
  assert.equal(isObstacleResolved(taken, '', 'Web app'), false);
  assert.equal(isObstacleResolved(taken, ' Web app ', 'Web app'), false);
  assert.equal(isObstacleResolved(taken, 'Web app 2', 'Web app'), true);
  assert.equal(isObstacleResolved({ code: 'entry_types.hub_bound', params: {} }, 'Other', 'Web app'), false);
});
