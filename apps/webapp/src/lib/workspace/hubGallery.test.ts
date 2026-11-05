// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { readdirSync } from 'node:fs';

import { ICON_NAMES } from '@hubtask/design-system/icons';

import { createsOf, familiesOf, familyOf, hubFamilies, markOf, prefilledName, switchedOn } from './hubGallery.ts';

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


test('choosing a template fills in its name until the reader types one', () => {
  assert.equal(prefilledName('', '', 'Scrum team'), 'Scrum team');
  assert.equal(prefilledName('Scrum team', 'Scrum team', 'Kanban team'), 'Kanban team', 'another choice replaces a filled-in name');
  assert.equal(prefilledName('Mobile app', 'Scrum team', 'Kanban team'), 'Mobile app', 'a typed name stays');
  assert.equal(prefilledName('Scrum team', 'Scrum team', ''), '', 'unchoosing takes the filled-in name away');
  assert.equal(prefilledName('  ', '', 'Scrum team'), 'Scrum team');
});

// §5.2: the catalogue filters by family, so a newcomer finds the service templates among thirty;
// a register stands in its own family, not under *More from Hubtask*.
test('the catalogue\'s families are the ones its templates fall in, in the gallery\'s order', () => {
  const list = [of('it_asset_register', 'HUBTASK', false), of('itil_service_desk'), of('mine', 'WORKSPACE'), of('household')];
  assert.equal(familyOf(of('vendor_contracts', 'HUBTASK', false)), 'registers');
  assert.deepEqual(familiesOf(list), ['personal', 'service', 'registers', 'workspace']);
});

// §5.2: a card's *Preview* says what taking it creates, in words - the walk found only a sentence.
test('a template says what it creates, per part, each block once, from every part it holds', () => {
  const template = {
    document: {
      modules: { planning: { switch: 'ON' } },
      types: { story: { name: 'Story' }, bug: { name: 'Bug' } },
      workflows: { story: { name: 'Story' } },
      roles: { product_owner: { name: 'Product Owner' } },
      fields: { gone: null },
    },
    hub_document: { views: { mine: { name: 'Mine' } }, template_pages: { how: { title: 'How we work' } }, rules: { nudge: {} } },
    collection_document: { types: { story: { name: 'Story again' } } },
  };
  assert.deepEqual(createsOf(template), [
    { part: 'types', names: ['Story', 'Bug'] },
    { part: 'workflows', names: ['Story'] },
    { part: 'roles', names: ['Product Owner'] },
    { part: 'views', names: ['Mine'] },
    { part: 'rules', names: ['nudge'] },
    { part: 'template_pages', names: ['How we work'] },
  ]);
  assert.deepEqual(createsOf({}), []);
});

// §5.2: a card's mark tells the templates apart - every shipped template has a drawing of its own
// from the icon set, none shared within its family; another origin's takes its family's.
test('every shipped template is drawn by a mark of its own', () => {
  const shipped = readdirSync(new URL('../../../../../core/application/service/config/shipped/', import.meta.url))
    .filter((name) => name.endsWith('.json')).map((name) => name.replace(/\.json$/u, ''));
  assert.ok(shipped.length >= 30, `read ${shipped.length} shipped templates`);
  const byFamily = new Map<string, Set<string>>();
  for (const key of shipped) {
    const template = { key, origin: 'HUBTASK' as const, hub_document: {} };
    const mark = markOf(template);
    assert.ok((ICON_NAMES as readonly string[]).includes(mark), `${key}: ${mark} is no icon`);
    assert.notEqual(mark, 'layout-template', `${key} has no mark of its own`);
    const family = familyOf(template);
    const seen = byFamily.get(family) ?? new Set<string>();
    assert.ok(!seen.has(mark), `${key} shares ${mark} within ${family}`);
    byFamily.set(family, seen.add(mark));
  }
  assert.equal(markOf({ key: 'itil_service_desk', origin: 'WORKSPACE' }), 'layout-template', 'a workspace\'s own template has no family of Hubtask');
});
