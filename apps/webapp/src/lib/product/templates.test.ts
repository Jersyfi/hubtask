// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { afterTaking, productTemplates } from './templates.ts';

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

// user-experience.md §3.2: after a template is taken, the app opens the home of its main product
// - the desk for a service template, the hub's screen of it where a hub took it.
test('a taken template opens its main product\'s home', () => {
  const template = (modules: Record<string, string>) =>
    ({ key: 'k', origin: 'HUBTASK', document: { modules: Object.fromEntries(Object.entries(modules).map(([key, value]) => [key, { switch: value }])) } });
  assert.equal(afterTaking(template({ workflows: 'ON', service_desk: 'ON', knowledge: 'ON' })), '/desk');
  assert.equal(afterTaking(template({ workflows: 'ON', service_desk: 'ON', knowledge: 'ON' }), 'h-1'), '/hubs/h-1/desk');
  assert.equal(afterTaking(template({ workflows: 'ON', planning: 'ON' })), '/plans');
  assert.equal(afterTaking(template({ workflows: 'ON' })), undefined, 'no product of its own: the page stays');
  assert.equal(afterTaking(template({ service_desk: 'OFF' })), undefined);
});
