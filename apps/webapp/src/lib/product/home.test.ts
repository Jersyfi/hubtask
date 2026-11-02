// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SOURCE } from '../i18n/catalogue.ts';
import { PRODUCTS } from '../navigation.ts';
import { ROUTES } from '../routes.ts';
import { resolve } from '../router.ts';
import { homeTabs, hubScreenOf, moreInHubtask, productDoors } from './home.ts';

// user-experience.md §2.2: every product's home leads somewhere - to a hub's screen of it or to a
// workspace-wide screen it already has - and every address resolves to a route.
test('every product home has a way in, and every address it offers is a route', () => {
  for (const row of PRODUCTS) {
    if (row.module === 'TIME' || row.module === 'GOALS') continue; // their own pages are the home
    const doors = productDoors(row.module, { isAdministrationReachable: true });
    const hub = hubScreenOf(row.module, '01a0e2e0-0000-7000-8000-000000000002');
    assert.ok(doors.length > 0 || hub !== undefined, `${row.module}'s home leads nowhere`);
    for (const path of [...doors.map((door) => door.path), ...(hub ? [hub] : []), ...homeTabs(row.module).map((tab) => tab.path)]) {
      assert.ok(resolve(ROUTES, path).name, `${row.module} offers ${path}, which is no route`);
    }
    for (const code of [...doors.map((door) => door.code), ...homeTabs(row.module).map((tab) => tab.code)]) {
      assert.ok(code in SOURCE, code);
    }
  }
});

test('the administration doors are offered only to whoever reaches the administration', () => {
  for (const row of PRODUCTS) {
    const paths = productDoors(row.module, { isAdministrationReachable: false }).map((door) => door.path);
    assert.ok(paths.every((path) => resolve(ROUTES, path).area !== 'administration'), `${row.module} offers ${paths.join(', ')}`);
  }
});

test('the pages home holds My notes as its second tab, at its own address', () => {
  assert.deepEqual(homeTabs('KNOWLEDGE').map((tab) => tab.path), ['/pages', '/notes']);
  assert.deepEqual(homeTabs('WHITEBOARD'), []);
});

// user-experience.md §3.3: the switched-off products are offered to whoever may switch one on, and
// to nobody else; nothing while the manifest is unread.
test('More in Hubtask lists the products that are off, for a holder of STRUCTURE only', () => {
  assert.deepEqual(moreInHubtask({ modulesOn: ['KNOWLEDGE', 'TIME'], mayShape: true }).map((row) => row.module), ['PLANNING', 'WHITEBOARD', 'SERVICE_DESK', 'ASSETS', 'GOALS', 'AGENTS']);
  assert.deepEqual(moreInHubtask({ modulesOn: [], mayShape: false }), []);
  assert.deepEqual(moreInHubtask({ modulesOn: undefined, mayShape: true }), []);
  assert.deepEqual(moreInHubtask({ modulesOn: PRODUCTS.map((row) => row.module), mayShape: true }), []);
});
