// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { CustomFieldDefinition, EffectiveStructure } from '@hubtask/sync-engine';

import { countsChange, figureOffer, isComplete } from './figureOffer.ts';

function structure(
  levels: string[],
  capabilities: Record<string, string[]>,
  extra: Partial<EffectiveStructure> = {},
): EffectiveStructure {
  const fields: Record<string, { capabilities: string[] }> = {};
  for (const [type, list] of Object.entries(capabilities)) fields[type] = { capabilities: list };
  return {
    preset: null,
    settings: { levels: levels.map((type) => ({ type, name: null })), fields, counts: { figure: 'OPEN', field_key: null } },
    sources: { counts: 'HUB' },
    locks: {},
    ...extra,
  } as unknown as EffectiveStructure;
}

const number = { key: 'amount', kind: 'NUMBER', applies_to: ['TASK'] } as unknown as CustomFieldDefinition;
const text = { key: 'shop', kind: 'TEXT', applies_to: ['TASK'] } as unknown as CustomFieldDefinition;

test('a list - one level - is never offered progress (UC-WRK-29 check 2)', () => {
  const offer = figureOffer(structure(['TASK'], { TASK: ['COMPLETION', 'CUSTOM_FIELDS'] }), [number]);
  assert.ok(!offer.figures.includes('PROGRESS'));
  assert.deepEqual(offer.figures, ['OPEN', 'FIELD_SUM', 'RHYTHM', 'FLOW', 'NONE']);
});

test('a project is offered all seven, in order', () => {
  const all = ['COMPLETION', 'DUE_DATE', 'CUSTOM_FIELDS'];
  const offer = figureOffer(structure(['TASK', 'WORK_PACKAGE', 'ACTIVITY'], { TASK: all, WORK_PACKAGE: all, ACTIVITY: ['COMPLETION'] }), [number]);
  assert.deepEqual(offer.figures, ['OPEN', 'FIELD_SUM', 'DUE', 'RHYTHM', 'PROGRESS', 'FLOW', 'NONE']);
});

test('due needs a level with a due date; a sum needs custom fields and a number field', () => {
  const undated = figureOffer(structure(['TASK'], { TASK: ['COMPLETION', 'CUSTOM_FIELDS'] }), [text]);
  assert.ok(!undated.figures.includes('DUE'));
  assert.ok(!undated.figures.includes('FIELD_SUM'), 'a text field is not summed');
  const noFields = figureOffer(structure(['TASK'], { TASK: ['COMPLETION', 'DUE_DATE'] }), [number]);
  assert.ok(noFields.figures.includes('DUE'));
  assert.ok(!noFields.figures.includes('FIELD_SUM'), 'a level without custom fields sums nothing');
  assert.deepEqual(figureOffer(structure(['TASK'], { TASK: ['CUSTOM_FIELDS'] }), [number, text]).fields, [{ key: 'amount', name: 'amount' }]);
});

test('a lock and a clip are named for the control to explain', () => {
  const locked = figureOffer(
    structure(['TASK'], { TASK: ['COMPLETION'] }, {
      locks: { counts: { origin: 'WORKSPACE', locked_by: null } },
      counts_clipped: { from: { figure: 'PROGRESS', field_key: null }, reason: 'ONE_LEVEL' },
    } as Partial<EffectiveStructure>),
    [],
  );
  assert.equal(locked.lockedBy, 'WORKSPACE');
  assert.equal(locked.clippedFrom, 'PROGRESS');
  assert.equal(locked.source, 'HUB');
  assert.equal(locked.current, 'OPEN');
});

test('a sum writes its field, and is not complete without one', () => {
  assert.deepEqual(countsChange('FIELD_SUM', 'amount'), { figure: 'FIELD_SUM', field_key: 'amount' });
  assert.deepEqual(countsChange('DUE', 'amount'), { figure: 'DUE' });
  assert.equal(isComplete('FIELD_SUM', undefined), false);
  assert.equal(isComplete('OPEN', undefined), true);
});
