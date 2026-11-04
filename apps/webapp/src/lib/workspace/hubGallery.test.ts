// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { afterCreating, hubFamilies, switchedOn } from './hubGallery.ts';

const of = (key: string, origin: 'HUBTASK' | 'INSTALLATION' | 'WORKSPACE' | 'OWN' = 'HUBTASK', hub = true) => ({
  key, origin, hub_document: hub ? {} : null,
});

// user-experience.md §5.2: creating a hub shows the template gallery grouped by family; a template
// without a hub part (a register, a preset's structure) is no hub's to take.
test('the hub gallery groups the templates with a hub part by family, in the catalogue order', () => {
  const groups = hubFamilies([
    of('itil_service_desk'),
    of('kanban_team'),
    of('household'),
    of('it_asset_register', 'HUBTASK', false),
    of('tasks', 'HUBTASK', false),
    of('ws_ours', 'WORKSPACE'),
    of('classic_project'),
    of('scrum_team'),
    of('something_new'),
  ]);
  assert.deepEqual(groups.map((group) => [group.family, group.templates.map((each) => each.key)]), [
    ['personal', ['household']],
    ['teams', ['kanban_team', 'scrum_team']],
    ['projects', ['classic_project']],
    ['service', ['itil_service_desk']],
    ['workspace', ['ws_ours']],
    ['other', ['something_new']],
  ]);
});

test('a card names the products a template switches on, workflows left out', () => {
  assert.deepEqual(switchedOn({ document: { modules: { workflows: { switch: 'ON' }, service_desk: { switch: 'ON' }, goals: { switch: 'OFF' }, knowledge: { switch: 'ON' } } } }),
    ['SERVICE_DESK', 'KNOWLEDGE']);
  assert.deepEqual(switchedOn({}), []);
});

test('a hub made from a chosen template opens on that template\'s plan, else where the caller goes', () => {
  assert.equal(afterCreating('h1', 'itil_service_desk', '/hubs/h1'), '/hubs/h1/settings/modules?template=itil_service_desk');
  assert.equal(afterCreating('h1', undefined, '/hubs/h1'), '/hubs/h1');
});
