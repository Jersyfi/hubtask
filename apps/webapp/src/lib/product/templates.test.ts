// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { productTemplates } from './templates.ts';

const of = (key: string, modules: Record<string, string>, origin = 'HUBTASK', hub = true) => ({
  key,
  origin,
  document: { modules: Object.fromEntries(Object.entries(modules).map(([name, on]) => [name, { switch: on }])) },
  hub_document: hub ? {} : null,
});

// A product's first visit offers the catalogue's templates that switch its module on (user-experience.md
// §4), Hubtask's first, at most three, and only those a hub can take - one mechanism, no second list.
test('a product offers the templates that switch its module on, Hubtask\'s first, at most three', () => {
  const list = [
    of('own_desk', { service_desk: 'ON' }, 'WORKSPACE'),
    of('itil_service_desk', { workflows: 'ON', service_desk: 'ON' }),
    of('internal_it', { workflows: 'ON', service_desk: 'ON' }),
    of('kanban_team', { workflows: 'ON' }),
    of('hr_requests', { workflows: 'ON', service_desk: 'ON' }),
    of('facility_requests', { workflows: 'ON', service_desk: 'ON' }),
    of('workspace_only', { service_desk: 'ON' }, 'HUBTASK', false),
    of('desk_off', { service_desk: 'OFF' }),
  ];
  assert.deepEqual(productTemplates(list, 'SERVICE_DESK').map((each) => each.key), ['itil_service_desk', 'internal_it', 'hr_requests']);
  assert.deepEqual(productTemplates(list, 'GOALS'), []);
});

test('the workflows every template switches on is no product of its own here', () => {
  assert.deepEqual(productTemplates([of('kanban_team', { workflows: 'ON' })], 'WORKFLOWS'), []);
});
