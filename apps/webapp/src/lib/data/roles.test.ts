// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { AccessExplanation, RoleCatalogue } from '@hubtask/sync-engine';

import { chosen, holds, keyFrom, matrixOf, offers } from './roles.ts';

const LUKAS = '22222222-0000-4000-8000-00000000000c';

const CATALOGUE: RoleCatalogue = {
  catalogue: [
    { permission: 'READ', kind: 'BASE', group: 'content', shipped: ['OWNER', 'ADMIN', 'MEMBER'] },
    { permission: 'STRUCTURE', kind: 'BASE', group: 'how_we_work', shipped: ['OWNER', 'ADMIN'] },
    { permission: 'DELETE_CONTAINER', kind: 'BASE', group: 'how_we_work', shipped: ['OWNER', 'ADMIN'] },
    { permission: 'MANAGE_MEMBERS', kind: 'BASE', group: 'people', shipped: ['OWNER', 'ADMIN'] },
    { permission: 'FIELD_WRITE:*', kind: 'FIELD', group: 'fields', shipped: ['OWNER', 'ADMIN'] },
    { permission: 'TAKE_STEP:*', kind: 'STEP', group: 'workflows', shipped: ['OWNER', 'ADMIN'] },
  ],
  shipped: [
    { role: 'OWNER', permissions: [] },
    { role: 'ADMIN', permissions: [] },
    { role: 'MEMBER', permissions: [] },
  ],
  custom: [
    {
      id: '33333333-0000-4000-8000-000000000001', key: 'service_lead', name: 'Service lead',
      permissions: ['READ', 'STRUCTURE', 'TAKE_STEP:change/approve'], version: 1,
      created_at: '2026-10-01T00:00:00Z', updated_at: '2026-10-01T00:00:00Z',
    },
  ],
  grants: [{ id: '44444444-0000-4000-8000-000000000001', role: 'service_lead', scope_type: 'HUB', account_id: LUKAS }],
};

function explained(permissions: string[], sources: AccessExplanation['sources']): AccessExplanation {
  return { account_id: LUKAS, scope_type: 'TENANT', roles: sources.map((source) => source.role), permissions, sources };
}

const ADMIN = explained(['READ', 'STRUCTURE', 'DELETE_CONTAINER', 'MANAGE_MEMBERS'], [{ role: 'ADMIN', custom: false, scope_type: 'TENANT' }]);
const LEAD = explained(['READ', 'STRUCTURE', 'MANAGE_MEMBERS', 'TAKE_STEP:change/approve'], [{ role: 'people_lead', custom: true, scope_type: 'TENANT' }]);
const OBJECTS = { restrictedFields: ['cost'], steps: [{ workflow: 'change', step: 'approve' }, { workflow: 'change', step: 'close' }] };

// UC-ID-26 check 6: an administrator is offered every permission, each restricted field's and each
// step's in place of their family.
test('an administrator is offered the whole catalogue, the families spelled out', () => {
  const offered = offers(CATALOGUE.catalogue, OBJECTS, ADMIN).map((offer) => offer.permission);
  assert.deepEqual(offered, [
    'READ', 'STRUCTURE', 'DELETE_CONTAINER', 'MANAGE_MEMBERS',
    'FIELD_WRITE:cost', 'TAKE_STEP:change/approve', 'TAKE_STEP:change/close',
  ]);
});

// UC-ID-26 checks 3 and 6: nobody is offered a permission they do not hold.
test('somebody holding a custom role is offered only what they hold', () => {
  const offered = offers(CATALOGUE.catalogue, OBJECTS, LEAD).map((offer) => offer.permission);
  assert.deepEqual(offered, ['READ', 'STRUCTURE', 'MANAGE_MEMBERS', 'TAKE_STEP:change/approve']);
  assert.equal(holds(LEAD, 'FIELD_WRITE:cost'), false);
  assert.equal(holds(ADMIN, 'FIELD_WRITE:cost'), true);
  assert.equal(holds(undefined, 'READ'), false);
});

// UC-ID-26 check 3: what the role carries beyond the reader's own is shown locked and kept on save.
test('a carried permission the reader lacks is locked and kept', () => {
  const carried = ['READ', 'DELETE_CONTAINER'];
  const offered = offers(CATALOGUE.catalogue, OBJECTS, LEAD, carried);
  const locked = offered.find((offer) => offer.permission === 'DELETE_CONTAINER');
  assert.equal(locked?.isHeld, false);
  assert.deepEqual(chosen(offered, new Set(['STRUCTURE', 'DELETE_CONTAINER']), carried), ['STRUCTURE', 'DELETE_CONTAINER']);
  assert.deepEqual(chosen(offered, new Set(['STRUCTURE']), carried), ['STRUCTURE', 'DELETE_CONTAINER']);
});

test('a key is proposed from a name, or nothing', () => {
  assert.equal(keyFrom('Service lead'), 'service_lead');
  assert.equal(keyFrom('Änderungs-Manager'), 'anderungs_manager');
  assert.equal(keyFrom('42'), '');
});

// UC-ID-26 check 7: the matrix is the catalogue against every role, narrowed to a person's roles.
test('who may do what draws the catalogue against the roles', () => {
  const matrix = matrixOf(CATALOGUE);
  assert.deepEqual(matrix.columns, [
    'READ', 'STRUCTURE', 'DELETE_CONTAINER', 'MANAGE_MEMBERS', 'FIELD_WRITE:*', 'TAKE_STEP:*', 'TAKE_STEP:change/approve',
  ]);
  const row = (id: string) => matrix.rows.find((each) => each.id === id)?.permissions;
  assert.deepEqual(row('MEMBER'), ['READ']);
  assert.ok(row('ADMIN')?.includes('TAKE_STEP:change/approve'));
  assert.deepEqual(row('service_lead'), ['READ', 'STRUCTURE', 'TAKE_STEP:change/approve']);
  assert.deepEqual(matrixOf(CATALOGUE, ['service_lead']).rows.map((each) => each.id), ['service_lead']);
});
