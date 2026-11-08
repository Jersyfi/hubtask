// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { settingsOf, startsOf, switchedOn } from './switchedon.ts';

const MODULES = ['WORKFLOWS', 'PLANNING', 'KNOWLEDGE', 'WHITEBOARD', 'SERVICE_DESK', 'ASSETS', 'TIME', 'GOALS', 'AGENTS'];

test('every module switched on for the workspace says where to find it or what to set up', () => {
  for (const module of MODULES) {
    const next = switchedOn(module);
    assert.ok(next.find !== undefined || next.setup !== undefined, module);
  }
  assert.deepEqual(switchedOn('WHITEBOARD').find, { code: 'app.nav.whiteboards', href: '/whiteboards' });
  assert.equal(switchedOn('SERVICE_DESK').setup?.href, '/desk/settings');
});

test('switched on at a hub, it is found on the hub\'s own tab', () => {
  assert.deepEqual(switchedOn('KNOWLEDGE', 'h').find, { code: 'app.pages.title', href: '/hubs/h/pages' });
  assert.equal(switchedOn('SERVICE_DESK', 'h').find?.href, '/hubs/h/desk');
  // A module the hub's strip has no tab for is found at its home.
  assert.equal(switchedOn('AGENTS', 'h').find?.href, '/agents');
});

// The Modules screen links each product's settings beside its switch (user-experience.md §8.2),
// only for a reader the settings tab is offered to (P-05).
test('a module with settings at its home names them, to whoever that tab is offered to', () => {
  const all = (): boolean => true;
  const none = (): boolean => false;
  assert.equal(settingsOf('SERVICE_DESK', none), '/desk/settings');
  assert.equal(settingsOf('ASSETS', none), '/assets/settings');
  assert.equal(settingsOf('TIME', all), '/time?tab=settings');
  assert.equal(settingsOf('TIME', none), undefined);
  assert.equal(settingsOf('TIME', (permission) => permission === 'APPROVE_TIME'), '/time?tab=settings');
  assert.equal(settingsOf('AGENTS', all), '/agents?tab=settings');
  assert.equal(settingsOf('AGENTS', (permission) => permission === 'STRUCTURE'), undefined);
  for (const module of ['WORKFLOWS', 'PLANNING', 'KNOWLEDGE', 'WHITEBOARD', 'GOALS']) assert.equal(settingsOf(module, all), undefined, module);
});

// §2.8: the card's *Start with…* is the templates that switch the module on, one press each - at
// a hub the press opens that template's plan on the hub's *Products*, at the workspace it is a
// new hub made from it (no link: the press opens *Create hub*).
test('the card starts with the templates that need the module, linked to the hub\'s plan there', () => {
  const desk = { key: 'itil_service_desk', origin: 'HUBTASK', hub_document: {}, document: { modules: { service_desk: { switch: 'ON' } } } };
  const plain = { key: 'kanban_team', origin: 'HUBTASK', hub_document: {}, document: { modules: { workflows: { switch: 'ON' } } } };
  assert.deepEqual(startsOf('SERVICE_DESK', [plain, desk]).map(({ template, href }) => [template.key, href]), [['itil_service_desk', undefined]]);
  assert.deepEqual(startsOf('SERVICE_DESK', [desk], 'h 1').map(({ href }) => href), ['/hubs/h%201/settings/template?template=itil_service_desk']);
  assert.deepEqual(startsOf('WHITEBOARD', [plain, desk]), []);
});
