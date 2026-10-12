// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import test from 'node:test';
import assert from 'node:assert/strict';

import type { InstanceSetting, InstanceSettings } from '@hubtask/sync-engine';

import { countOf, documentOf, lockCode, rowsOf } from './values.ts';

const count = (extra: Partial<InstanceSetting> = {}): InstanceSetting => ({
  set: false, locked: false, kind: 'COUNT', minimum: 8, maximum: 128, workspace_may: 'TIGHTEN', ...extra,
});

test('a count is read as a number, and anything else as it was typed, for the server to refuse', () => {
  assert.equal(countOf('12'), 12);
  assert.equal(countOf(' 0 '), 0);
  assert.equal(countOf(''), undefined);
  assert.equal(countOf('3.5'), '3.5');
  assert.equal(countOf('twelve'), 'twelve');
});

test('the lock is worded by what an open key leaves a workspace free to do', () => {
  assert.equal(lockCode(count(), false), 'app.instance.lock_tighten_open');
  assert.equal(lockCode(count({ workspace_may: 'RAISE' }), true), 'app.instance.lock_raise_locked');
  assert.equal(lockCode(count({ workspace_may: 'REPLACE' }), false), 'app.instance.lock_replace_open');
  assert.equal(lockCode({ set: false, locked: false, kind: 'LOCALE' }, false), undefined);
});

test('what was decided is sent in its kind, and what was not is left out', () => {
  const draft: InstanceSettings = {
    sign_in: {
      min_length: count({ set: true, value: 14, locked: true }),
      min_digits: count(),
    },
    quotas: { items: { set: true, value: 500, locked: false, kind: 'COUNT', minimum: 0, workspace_may: 'RAISE' } },
    blocklist_file: '',
  };
  const document = documentOf(draft);
  assert.deepEqual(document.sign_in, { min_length: { set: true, value: 14, locked: true } });
  assert.deepEqual(document.quotas, { items: { set: true, value: 500, locked: false } });
  assert.deepEqual(document.legal, {});
});

test('the areas are drawn in the screen order, each key the server answered', () => {
  const rows = rowsOf({
    ai: { sources: { set: false, locked: false, kind: 'CHOICE', options: ['EITHER'] } },
    sign_in: { min_length: count() },
  });
  assert.deepEqual(rows.map((row) => `${row.area}.${row.key}`), ['sign_in.min_length', 'ai.sources']);
});
