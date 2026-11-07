// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { AccessExplanation, RoleCatalogue } from '@hubtask/sync-engine';

import { chosen, holds, keyFrom, matrixOf, mayReadField, mayWriteField, offers, settingWritersOf } from './roles.ts';

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

// UC-ID-27 check 4: the entry panel offers a restricted field only to whoever holds its permission,
// and nobody before the answer has arrived.
test('a restricted field is written only with its permission', () => {
  const cost = { key: 'cost', restricted: true };
  assert.equal(mayWriteField({ key: 'title_note' }, undefined), true);
  assert.equal(mayWriteField(cost, undefined), false);
  assert.equal(mayWriteField(cost, LEAD), false);
  assert.equal(mayWriteField(cost, ADMIN), true);
  const finance = explained(['READ', 'FIELD_WRITE:cost'], [{ role: 'finance', custom: true, scope_type: 'HUB' }]);
  assert.equal(mayWriteField(cost, finance), true);
});

test('a field hidden from readers is seen only with its read or write permission (UC-ID-27 check 3)', () => {
  const cost = { key: 'cost', restricted: true, read_restricted: true };
  assert.equal(mayReadField({ key: 'severity' }, undefined), true);
  assert.equal(mayReadField({ key: 'cost', read_restricted: false }, undefined), true);
  assert.equal(mayReadField(cost, undefined), false);
  assert.equal(mayReadField(cost, LEAD), false);
  assert.equal(mayReadField(cost, ADMIN), true);
  const reader = explained(['READ', 'FIELD_READ:cost'], [{ role: 'controlling', custom: true, scope_type: 'HUB' }]);
  assert.equal(mayReadField(cost, reader), true);
  assert.equal(mayWriteField(cost, reader), false);
  const writer = explained(['READ', 'FIELD_WRITE:cost'], [{ role: 'finance', custom: true, scope_type: 'HUB' }]);
  assert.equal(mayReadField(cost, writer), true);
});

test('a role editor offers the read permission of each hidden field', () => {
  const catalogue: RoleCatalogue['catalogue'] = [
    { permission: 'FIELD_WRITE:*', kind: 'FIELD', group: 'fields', shipped: ['OWNER', 'ADMIN'] },
    { permission: 'FIELD_READ:*', kind: 'FIELD', group: 'fields', shipped: ['OWNER', 'ADMIN'] },
  ];
  const offered = offers(catalogue, { restrictedFields: ['cost', 'band'], hiddenFields: ['cost'], steps: [] }, ADMIN)
    .map((offer) => offer.permission);
  assert.deepEqual(offered, ['FIELD_WRITE:cost', 'FIELD_WRITE:band', 'FIELD_READ:cost']);
});

// ui.md A11: who may change which setting, at the workspace's levels only, by shipped and own roles.
test('a setting is answered with the roles that carry its writer, at the workspace and below', () => {
  const settings = [
    {
      key: 'structure.views',
      writers: [
        { level: 'INSTANCE', permission: 'admin:instance' },
        { level: 'WORKSPACE', permission: 'STRUCTURE' },
        { level: 'HUB', permission: 'STRUCTURE' },
      ],
    },
    { key: 'sign_in.min_length', writers: [{ level: 'WORKSPACE', permission: 'MANAGE_MEMBERS' }] },
    { key: 'localisation.locale', writers: [{ level: 'PERSON', permission: 'SELF' }] },
  ];
  assert.deepEqual(settingWritersOf(settings, CATALOGUE), [
    { key: 'structure.views', level: 'WORKSPACE', permission: 'STRUCTURE', roles: ['OWNER', 'ADMIN', 'service_lead'] },
    { key: 'structure.views', level: 'HUB', permission: 'STRUCTURE', roles: ['OWNER', 'ADMIN', 'service_lead'] },
    { key: 'sign_in.min_length', level: 'WORKSPACE', permission: 'MANAGE_MEMBERS', roles: ['OWNER', 'ADMIN'] },
  ]);
  assert.deepEqual(
    settingWritersOf(settings, CATALOGUE, ['service_lead']).map((row) => row.roles),
    [['service_lead'], ['service_lead'], []],
  );
});

// A part of another control (the rotation's start) is written with that control, and has no row
// and no name of its own - as the palette leaves it out.
test('a part of another control is not a setting of its own in who may change which setting', () => {
  const settings = [
    { key: 'sign_in.rotation_from', kind: 'STRUCTURE', writers: [{ level: 'WORKSPACE', permission: 'MANAGE_MEMBERS' }] },
    { key: 'structure.views', kind: 'STRUCTURE', writers: [{ level: 'WORKSPACE', permission: 'STRUCTURE' }] },
  ];
  assert.deepEqual(settingWritersOf(settings, CATALOGUE).map((row) => row.key), ['structure.views']);
});
