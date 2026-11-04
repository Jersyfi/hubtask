// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// ui.md A4: an editor opens the draft of its own block, and publishing waits on a plan that
// neither refuses nor hides data unacknowledged.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { draftOf, draftRows, draftWords, publishStep, removalDocument, removedBlock } from './configdrafts.ts';

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

test('Changes lists every drafted block, each opening the page that publishes it', () => {
  const list = { drafts: [draft('types', 'incident'), { ...draft('roles', 'lead'), section: 'roles' as const }, { ...draft('types', 'x'), section: 'sla_policies' as never, key: 'gold' }] };
  const rows = draftRows(list, [{ key: 'support', name: 'Support', draft: { states: [] } }, { key: 'done', draft: null }]);
  assert.deepEqual(rows.map((row) => [row.path, row.href]), [
    ['workflows.support', '/administration/workflows'],
    ['types.incident', '/administration/types'],
    ['roles.lead', '/administration/people?tab=permissions'],
    ['sla_policies.gold', '/desk/settings'],
  ]);
  assert.deepEqual(draftRows(undefined, []), []);
});

test('a draft reads as the draft of its section, by its name or else its key', () => {
  assert.deepEqual(draftWords({ path: 'workflows.support', name: 'Support', href: '/x' }),
    { code: 'app.changes.draft_of', section: 'app.templates.section_workflows', key: 'Support' });
  assert.deepEqual(draftWords({ path: 'sla_policies.gold', href: '/x' }),
    { code: 'app.changes.draft_of', section: 'app.templates.section_sla_policies', key: 'gold' });
  assert.equal(draftWords({ path: 'something_new.x', href: '/x' }).section, 'app.templates.section_other');
});

test('a role is deleted by planning its removal, and Undo plans back the block the plan took', () => {
  assert.deepEqual(removalDocument('roles', 'auditor'), { roles: { auditor: null } });
  const before = { name: 'Auditor', permissions: ['READ'] };
  const plan = { changes: [
    { path: 'roles.other', section: 'roles', key: 'other', op: 'REMOVE', fields: [], before: { name: 'Other' }, after: null },
    { path: 'roles.auditor', section: 'roles', key: 'auditor', op: 'REMOVE', fields: [], before, after: null },
  ] };
  assert.deepEqual(removedBlock(plan as never, 'roles', 'auditor'), before);
  assert.equal(removedBlock({ changes: [{ ...plan.changes[1], op: 'CHANGE' }] } as never, 'roles', 'auditor'), undefined);
});
