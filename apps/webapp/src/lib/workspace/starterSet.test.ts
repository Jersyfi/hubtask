// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { BUILT_IN } from '../data/presetChoice.ts';
import { afterStarter, PURPOSES, STARTER_SETS, starterFamilies, templateStarterBody } from './starterSet.ts';

const catalogue = JSON.parse(readFileSync(new URL('../../../../../locales/en.json', import.meta.url), 'utf8')) as Record<string, string>;

test('every purpose has a hub on a preset and named collections, each name in the catalogue (UC-WRK-30 check 5)', () => {
  for (const purpose of PURPOSES) {
    const set = STARTER_SETS[purpose];
    assert.ok((BUILT_IN as readonly string[]).includes(set.hub.preset), `${purpose}'s hub has no preset`);
    assert.ok(set.collections.length > 0, `${purpose} makes no collection`);
    for (const place of [set.hub, ...set.collections]) {
      assert.ok(catalogue[place.name], `${place.name} is not in the catalogue`);
      assert.ok(place.preset === undefined || (BUILT_IN as readonly string[]).includes(place.preset));
    }
    assert.ok(catalogue[`app.starter.purpose.${purpose}`], `${purpose} is not worded`);
  }
});

test('the starter offers Simple first, then each family the catalogue holds, a register included (§3.2)', () => {
  const families = starterFamilies([
    { key: 'itil_service_desk', origin: 'HUBTASK', hub_document: {} },
    { key: 'household', origin: 'HUBTASK', hub_document: {} },
    { key: 'it_asset_register', origin: 'HUBTASK', hub_document: null },
    { key: 'ours', origin: 'WORKSPACE', hub_document: {} },
  ] as const);
  assert.deepEqual(families.map((each) => each.family), ['simple', 'personal', 'service', 'registers', 'workspace']);
  assert.deepEqual(families.find((each) => each.family === 'service')?.templates.map((each) => each.key), ['itil_service_desk']);
  for (const each of families) assert.ok(catalogue[`app.workspace.hub_family.${each.family}`], `${each.family} is not worded`);
  for (const each of families.filter((one) => one.family !== 'workspace')) {
    assert.ok(catalogue[`app.starter.family_about.${each.family}`], `${each.family} has no sentence`);
  }
});

test('a set on a template names it, its answers and samples, and leaves the preset to the workspace (UC-WRK-30 check 5)', () => {
  assert.deepEqual(templateStarterBody('Service desk', 'itil_service_desk', { first_group: 'g1' }, true, false), {
    hub_name: 'Service desk', collections: [], template: 'itil_service_desk', answers: { first_group: 'g1' }, samples: true,
  });
  assert.deepEqual(templateStarterBody('Team', 'kanban_team', {}, false, true), {
    hub_name: 'Team', collections: [], template: 'kanban_team', acknowledge: true,
  });
});

test('after the set the app opens the template\'s main product in the hub, else the hub (§3.2)', () => {
  const desk = { document: { modules: { workflows: { switch: 'ON' }, service_desk: { switch: 'ON' }, knowledge: { switch: 'ON' } } } };
  assert.notEqual(afterStarter(desk, 'h1'), '/hubs/h1');
  assert.match(afterStarter(desk, 'h1'), /h1/);
  assert.equal(afterStarter({ document: { modules: { workflows: { switch: 'ON' } } } }, 'h1'), '/hubs/h1');
  assert.equal(afterStarter(undefined, 'h1'), '/hubs/h1');
});
