// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Preset } from '@hubtask/sync-engine';

import { isBuiltIn, presetChoice, presetValue } from './presetChoice.ts';

test("Hubtask's preset is named by its key, a saved one by its id (UC-WRK-30)", () => {
  assert.deepEqual(presetChoice('FLOW'), { preset: 'FLOW' });
  assert.deepEqual(presetChoice('01a0e2e0-0000-7000-8000-0000000000f1'), { preset_id: '01a0e2e0-0000-7000-8000-0000000000f1' });
  assert.deepEqual(presetChoice(''), {});
  assert.deepEqual(presetChoice(undefined), {});
  assert.equal(isBuiltIn('LIST'), true);
  assert.equal(isBuiltIn('list'), false);
  assert.equal(presetValue({ origin: 'HUBTASK', key: 'TASKS', settings: {} } as unknown as Preset), 'TASKS');
  assert.equal(presetValue({ origin: 'WORKSPACE', id: 'x1', name: 'Weekly shop', settings: {} } as unknown as Preset), 'x1');
});
