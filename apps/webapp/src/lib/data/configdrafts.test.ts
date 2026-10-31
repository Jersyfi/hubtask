// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// ui.md A4: an editor opens the draft of its own block, and publishing waits on a plan that
// neither refuses nor hides data unacknowledged.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { draftOf, publishStep } from './configdrafts.ts';

const draft = (section: 'types' | 'roles', key: string) => ({ section, key, block: {}, saved_by: 'a', saved_at: '2026-10-31T09:00:00Z', version: 1 });

test('an editor opens the draft of its own block only', () => {
  const list = { drafts: [draft('roles', 'incident'), draft('types', 'incident')] };
  assert.equal(draftOf(list, 'types', 'incident')?.section, 'types');
  assert.equal(draftOf(list, 'types', 'change'), undefined);
  assert.equal(draftOf(undefined, 'types', 'incident'), undefined);
});

test('a publish plan is held back by a refusal and by data it hides', () => {
  const flags = { hides_data: false, changes_permissions: false };
  const change = { path: 'types.incident', section: 'types', key: 'incident', op: 'CHANGE' };
  assert.equal(publishStep({ plan_hash: null, refusals: [{ path: 'types.incident', code: 'x' }], flags, changes: [] } as never), 'refused');
  assert.equal(publishStep({ plan_hash: 'h', refusals: [], flags, changes: [] } as never), 'unchanged');
  assert.equal(publishStep({ plan_hash: 'h', refusals: [], flags: { ...flags, hides_data: true }, changes: [change] } as never), 'acknowledge');
  assert.equal(publishStep({ plan_hash: 'h', refusals: [], flags, changes: [change] } as never), 'ready');
});
