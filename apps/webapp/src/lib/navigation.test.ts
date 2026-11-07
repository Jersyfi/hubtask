// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The one navigation list against the route table and the catalogue (ADR-0061 decision 1).
//
// What is worth asserting is not what the list says — that is read off the screen — but that it
// cannot disagree with the two things it points at: every route target resolves to a route the
// destination claims as its own, every word is a code the catalogue has, and the two areas
// ADR-0032 switches on are the route table's, not this file's opinion. And that the bottom bar
// can draw it: three to five, or the component refuses.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SOURCE } from './i18n/source.ts';
import GERMAN from '../../../../locales/de.json' with { type: 'json' };
import { ADMINISTRATION, DESTINATIONS, INSTANCE, KEEPING, MORE_CODE, PRODUCTS, TRASH, account, administration, administrationFor, bar, currentDestination, firstScreen, installationPlace, instanceSection, leftToWebApp, primary, products, workspaceBand, HUB_TOOLS, hubTools, switchPath, toolOf, toolPath, type SectionGroup } from './navigation.ts';
import { contentsCount, contentsTree, figureCount, foot, forYou, levelOf, navProducts, orderHubs, placedIndex, queuePath, queueRows, type LevelRoute } from './sidebar.ts';
import { ROUTES, movedTo } from './routes.ts';
import { resolve } from './router.ts';

test('every destination with a route lands on a route it claims as current', () => {
  for (const destination of DESTINATIONS) {
    if (destination.target.kind !== 'route') continue;
    const resolution = resolve(ROUTES, destination.target.path);
    assert.ok(resolution.name, `${destination.id} points at ${destination.target.path}, which resolves to nothing`);
    assert.ok(destination.routes.includes(resolution.name), `${destination.id} lands on ${resolution.name} but does not claim it`);
    assert.equal(currentDestination(resolution), destination.id);
  }
  for (const row of [...KEEPING, ...PRODUCTS, ...workspaceBand({ isAdministrationReachable: true })]) {
    const resolution = resolve(ROUTES, row.path);
    assert.ok(resolution.name, `${row.id} points at ${row.path}, which resolves to nothing`);
    assert.ok(row.routes.includes(resolution.name), `${row.id} lands on ${resolution.name} but does not claim it`);
    assert.equal(currentDestination(resolution), row.id);
  }
});

test('nothing is in two bands, and nothing is outside one', () => {
  // The five bands (user-experience.md §2.1): places, products, the tree (the containers, which
  // are data rather than a list here), the workspace's administration and `keeping`. A row in two
  // would be drawn twice, which is the failure the one list exists to prevent.
  const every = ['SERVICE_DESK', ...PRODUCTS.map((row) => row.module)];
  const bands = [
    primary({ hasJumble: true, hasCapture: true, modulesOn: every }).map((destination) => destination.id),
    products({ modulesOn: every }).map((row) => row.id),
    workspaceBand({ isAdministrationReachable: true }).map((row) => row.id),
    KEEPING.map((row) => row.id),
  ];
  const seen = new Set<string>();
  for (const band of bands) {
    for (const id of band) {
      assert.equal(seen.has(id), false, `${id} is in two bands`);
      seen.add(id);
    }
  }
  // The account group is behind the avatar, not in the column: no band holds it.
  for (const destination of DESTINATIONS) {
    if (destination.group !== 'account') continue;
    assert.equal(seen.has(destination.id), false, `${destination.id} is in the column`);
  }
});

// user-experience.md §2.1: for every module with a home, on draws exactly one products row in the
// column, the drawer and the phone's *More* (all three draw `products`), off draws none.
test('a product is one row of the products band where its module is on, and none where it is off', () => {
  const keys = ['WORKFLOWS', 'PLANNING', 'KNOWLEDGE', 'WHITEBOARD', 'SERVICE_DESK', 'ASSETS', 'TIME', 'GOALS', 'AGENTS'];
  assert.deepEqual(PRODUCTS.map((row) => row.module), keys.filter((key) => key !== 'WORKFLOWS'), 'the band is not in Module.Keys() order');
  assert.deepEqual(products({ modulesOn: [] }), [], 'a household with nothing on sees a products row');
  assert.deepEqual(products({}), []);
  for (const key of keys) {
    const rows = products({ modulesOn: [key] });
    assert.equal(rows.length, key === 'WORKFLOWS' ? 0 : 1, `${key} draws ${rows.length} rows`);
    assert.equal(products({ modulesOn: keys.filter((each) => each !== key) }).some((row) => row.module === key), false, `${key} is drawn while off`);
  }
  for (const row of PRODUCTS) {
    assert.ok(row.code in SOURCE, row.code);
    assert.ok(row.words in SOURCE, row.words);
    for (const name of row.routes) assert.equal(currentDestination({ name, area: 'end-user' }), row.id, `${name} does not mark ${row.id}`);
  }
});

test('the administration is the workspace band, drawn only where the server says so', () => {
  assert.deepEqual(workspaceBand({ isAdministrationReachable: true }).map((row) => row.id), ['administration']);
  assert.deepEqual(workspaceBand({ isAdministrationReachable: false }), []);
  assert.equal(currentDestination({ name: 'modules', area: 'administration' }), 'administration');
});

test('every route a destination claims exists in the table', () => {
  const names = new Set(ROUTES.map((route) => route.name));
  for (const destination of DESTINATIONS) {
    for (const name of destination.routes) assert.ok(names.has(name), `${destination.id} claims ${name}, which no route is named`);
  }
});

test('the areas are the route table’s, not the list’s', () => {
  // A destination's `area` is what the mobile build reads; the route the destination lands on
  // is what the shell excludes. The two are the same fact and must say the same thing.
  for (const destination of DESTINATIONS) {
    if (destination.target.kind !== 'route') continue;
    assert.equal(resolve(ROUTES, destination.target.path).area, destination.area ?? 'end-user', destination.id);
  }
  assert.equal(DESTINATIONS.filter((destination) => destination.area === 'administration').length, 0, 'the administration left the account menu (§2.3)');
});

test('every word is a code the catalogue has', () => {
  for (const destination of DESTINATIONS) assert.ok(destination.code in SOURCE, destination.code);
  for (const row of KEEPING) assert.ok(row.code in SOURCE, row.code);
  assert.ok(TRASH.code in SOURCE);
  assert.ok(MORE_CODE in SOURCE);
  for (const destination of DESTINATIONS) if (destination.words) assert.ok(destination.words in SOURCE, destination.words);
});

test('the bottom bar can draw its places and "More"', () => {
  // `BottomBar` takes three to five destinations: Today, Search, Capture, the Inbox and *More*
  // (user-experience.md §2.6). Fewer is a switch, more is narrower than a thumb.
  for (const hasJumble of [true, false]) {
    for (const hasCapture of [true, false]) {
      const count = bar({ hasJumble, hasCapture }).length + 1;
      assert.ok(count >= 3 && count <= 5, `${count} destinations on a phone`);
    }
  }
  assert.deepEqual(bar({ hasJumble: true, hasCapture: true }).map((destination) => destination.id), ['workspace', 'search', 'capture', 'jumble']);
});

test('capture is an act in the primary group, drawn only where asked, between Search and the inbox', () => {
  // The column lists places; the bottom bar draws the ＋ (UC-WRK-26 check 1, ux-inventory.md §E0).
  assert.deepEqual(primary({ hasJumble: true }).map((destination) => destination.id), ['workspace', 'search', 'jumble']);
  assert.deepEqual(primary({ hasJumble: true, hasCapture: true }).map((destination) => destination.id), ['workspace', 'search', 'capture', 'jumble']);
  const capture = DESTINATIONS.find((destination) => destination.id === 'capture');
  assert.equal(capture?.target.kind, 'capture');
  assert.deepEqual(capture?.routes, []);
});

// UC-SVC-09 check 1, user-experience.md §2.1: what waits for the reader's yes is a place of the
// column where the service desk is on, and the drawer's on a phone.
test('the approvals are a place only where the service desk is on', () => {
  assert.ok(!primary({ hasJumble: true }).some((destination) => destination.id === 'approvals'));
  assert.deepEqual(primary({ hasJumble: true, modulesOn: ['SERVICE_DESK'] }).map((destination) => destination.id), ['workspace', 'search', 'jumble', 'approvals']);
  assert.ok(!bar({ hasJumble: true }).some((destination) => destination.id === 'approvals'), 'the bar carries the approvals');
});

test('the search exists once', () => {
  assert.equal(DESTINATIONS.filter((destination) => destination.target.kind === 'route' && destination.target.path === '/search').length, 1);
});

// user-experience.md §2.3: the account menu holds only what is about the person.
test('the account menu holds only what is about the person', () => {
  const ids = (reachable: boolean) =>
    account({ isAdministrationReachable: reachable, isInstanceReachable: false }).map((destination) => destination.id);
  assert.deepEqual(ids(true), ['profile', 'appearance', 'help', 'about', 'sign-out']);
  assert.deepEqual(ids(false), ['profile', 'appearance', 'help', 'about', 'sign-out']);
  const rows = (modulesOn?: readonly string[]) =>
    administration({ hasPrivateHubs: true, modulesOn }).flatMap((group) => group.rows.map((row) => row.id));
  assert.equal(rows().includes('entry-types'), false);
  assert.equal(rows(['WORKFLOWS']).includes('entry-types'), true);
  // UC-GOL-01 check 9: the goals row stays current on each of their pages.
  for (const name of ['goals', 'goal', 'portfolio', 'dashboard']) {
    assert.equal(currentDestination({ name, area: 'end-user' }), 'goals');
  }
});

test('Switch workspace is offered only where the device keeps a list, before the ways through (UC-ID-25 checks 2, 8)', () => {
  const ids = (hasWorkspaceList: boolean) =>
    account({ isAdministrationReachable: true, isInstanceReachable: false, hasWorkspaceList }).map((destination) => destination.id);
  assert.deepEqual(ids(true), ['profile', 'appearance', 'switch-workspace', 'help', 'about', 'sign-out']);
  assert.ok(!ids(false).includes('switch-workspace'), 'single mode was offered a list of workspaces');
  // The separator opens *Help*, not the switch: the switch belongs with the reader's own places.
  const banded = account({ isAdministrationReachable: true, isInstanceReachable: false, hasWorkspaceList: true })
    .filter((destination) => destination.startsBand)
    .map((destination) => destination.id);
  assert.deepEqual(banded, ['help']);
});

test('an installed app offers Switch workspace and Add a workspace, in single mode too (UC-ID-25 check 7, UC-SYN-07 check 4)', () => {
  const ids = account({ isAdministrationReachable: true, isInstanceReachable: false, isSingleWorkspace: true, holdsWorkspaces: true })
    .map((destination) => destination.id);
  assert.deepEqual(ids.slice(0, 4), ['profile', 'appearance', 'switch-workspace', 'add-workspace']);
  assert.ok(
    !account({ isAdministrationReachable: true, isInstanceReachable: false, hasWorkspaceList: true })
      .some((destination) => destination.id === 'add-workspace'),
    'the browser was offered to add a workspace by its address',
  );
});

test('the instance row is offered only to an operator, and is absent rather than disabled', () => {
  // The level above the workspaces is drawn for an account in the operator register and for
  // nobody else. Absent, not greyed — somebody who is not an operator was never going to have it,
  // and a row they cannot use is a row they have to ask about (ADR-0070 §5).
  const ids = (reachable: boolean) =>
    account({ isAdministrationReachable: true, isInstanceReachable: reachable }).map((destination) => destination.id);
  assert.deepEqual(ids(true), ['profile', 'appearance', 'instance', 'help', 'about', 'sign-out']);
  assert.ok(!ids(false).includes('instance'), 'a non-operator was offered the control plane');

  // And it is the row every screen of the area is current for, so the menu marks one place rather
  // than none while somebody is inside it.
  for (const route of ROUTES.filter((each) => each.area === 'instance')) {
    assert.equal(currentDestination({ name: route.name, area: 'instance' }), 'instance', route.name);
  }
});

test('the installation is still in the list, under another name, and signing out is last', () => {
  // It stopped being a destination called "This installation" and became "About Hubtask" near the
  // end of the menu (ADR-0063 decision 6) — a move, not a removal: the route is the same, the
  // page is the same, and every reader still reaches it. Parity (ADR-0032) is about what a
  // person can do, and nothing here is one thing fewer.
  const about = DESTINATIONS.find((destination) => destination.id === 'about');
  assert.equal(about?.target.kind === 'route' && about.target.path, '/installation');
  assert.deepEqual(about?.routes, ['installation']);
  assert.equal(about?.area, undefined, 'everyone may read what this installation is');
  // ADR-0065 decision 5 reverses decision 6's order: signing out is the last thing a reader does
  // in a session, and a row under it is a row somebody reaches past.
  const group = account({ isAdministrationReachable: false, isInstanceReachable: false });
  assert.equal(group.at(-1)?.id, 'sign-out', 'something is drawn under signing out');
  assert.equal(group.at(-2)?.id, 'about');
});

test('every screen under the administration is the administration destination', () => {
  for (const route of ROUTES.filter((each) => each.area === 'administration')) {
    assert.equal(currentDestination(resolve(ROUTES, route.pattern.replaceAll(/:\w+/g, 'x'))), 'administration', route.name);
  }
  // And a screen outside the list belongs to nothing: the bar marks no destination current.
  assert.equal(currentDestination(resolve(ROUTES, '/redeem')), undefined);
  assert.equal(currentDestination({ name: null, area: 'end-user' }), undefined);
});

test('a section\u2019s address opens its first screen, which is never the way out', () => {
  // The way back is a row with no route of its own, and a front door that led out of the section
  // would be a door somebody falls through. The administration's front door is its overview
  // (UC-ID-24 check 1): what needs attention, not the first settings page.
  const front = firstScreen(ADMINISTRATION);
  assert.equal(front, '/administration');
  const resolution = resolve(ROUTES, front);
  assert.equal(resolution.name, 'administration');
  assert.equal(resolution.area, 'administration');
  // And it is a row of the list, so the column marks it current the moment the reader lands.
  const rows = ADMINISTRATION.flatMap((group) => group.rows);
  assert.ok(rows.some((row) => row.path === front && row.routes.includes(resolution.name as string)));
  assert.equal(rows.find((row) => row.path === front)?.id !== 'back', true);
});

test('the administration is grouped in the suite\u2019s six groups (configuration/ui.md \u00a71)', () => {
  // the overview under the way out \u00b7 how we work \u00b7 people and access \u00b7 connections \u00b7 data
  // and records \u00b7 changes; every page one row away, so reachable in at most two steps (UC-ID-24 check 3).
  assert.deepEqual(ADMINISTRATION.map((group) => group.id), ['back', 'how', 'people', 'connections', 'data', 'changes']);
  const of = (id: string) => ADMINISTRATION.find((group) => group.id === id)?.rows.map((row) => row.id);
  assert.deepEqual(of('back'), ['back', 'overview']);
  // The desk's, the assets', time's and the agents' settings are their homes' last tabs (user-experience.md §8.2).
  assert.deepEqual(of('how'), ['workspace', 'address', 'presets', 'modules', 'entry-types', 'workflows', 'link-types', 'rules', 'runs']);
  assert.deepEqual(of('people'), ['people', 'sign-in-settings', 'legal', 'machines']);
  assert.deepEqual(of('connections'), ['mail', 'webhooks', 'voice', 'ai']);
  assert.deepEqual(of('data'), ['private-hubs', 'backup', 'retention', 'restore', 'audit', 'privacy', 'quotas']);
  assert.deepEqual(of('changes'), ['changes']);
  for (const group of ADMINISTRATION) {
    if (group.code) assert.ok(group.code in SOURCE, group.code);
    for (const row of group.rows) assert.ok(row.code in SOURCE, row.code);
  }
});

// UC-FLW-01 check 8, UC-INS-21 check 4: a module's page is drawn only where the module is on.
test('the types page is drawn only where the workflows module is on', () => {
  const rows = (modulesOn?: readonly string[]) =>
    administration({ hasPrivateHubs: true, modulesOn }).flatMap((group) => group.rows.map((row) => row.id));
  assert.equal(rows().includes('entry-types'), false);
  assert.equal(rows(['PLANNING']).includes('entry-types'), false);
  assert.equal(rows(['WORKFLOWS']).includes('entry-types'), true);
});

test('every row of the section resolves to a route it claims, and every screen is reachable', () => {
  for (const group of ADMINISTRATION) {
    for (const row of group.rows) {
      const resolution = resolve(ROUTES, row.path);
      assert.ok(resolution.name, `${row.id} points at ${row.path}, which resolves to nothing`);
      if (row.routes.length === 0) continue;
      assert.ok(row.routes.includes(resolution.name), `${row.id} lands on ${resolution.name} but does not claim it`);
    }
  }
  // Every screen of the area has a row, so the column is the whole of the section (ADR-0065
  // decision 1): with the index gone, a screen nothing lists is a screen nobody can reach.
  const claimed = new Set(ADMINISTRATION.flatMap((group) => group.rows).flatMap((row) => row.routes));
  // An address whose screen moved elsewhere (`movedTo`) replaces itself and needs no row.
  const unlisted = ROUTES.filter((route) => route.area === 'administration')
    .map((route) => route.name)
    .filter((name) => !claimed.has(name) && movedTo(name) === undefined);
  assert.deepEqual(unlisted, [], `these screens are in the area and in nobody's list: ${unlisted.join(', ')}`);
});

test('the installation section lists every screen of its area, and every word of it', () => {
  // The same two invariants the administration has, for the same two reasons: with no index screen
  // the column *is* the section (ADR-0065 decision 1), so a screen nothing lists is a screen nobody
  // reaches.
  for (const group of INSTANCE) {
    for (const row of group.rows) {
      assert.ok(row.code in SOURCE, row.code);
      const resolution = resolve(ROUTES, row.path);
      assert.ok(resolution.name, `${row.id} points at ${row.path}, which resolves to nothing`);
      if (row.routes.length === 0) continue;
      assert.ok(row.routes.includes(resolution.name), `${row.id} lands on ${resolution.name} but does not claim it`);
    }
  }
  const claimed = new Set(INSTANCE.flatMap((group) => group.rows).flatMap((row) => row.routes));
  const unlisted = ROUTES.filter((route) => route.area === 'instance')
    .map((route) => route.name)
    .filter((name) => !claimed.has(name));
  assert.deepEqual(unlisted, [], `these screens are in the area and in nobody's list: ${unlisted.join(', ')}`);

  // And its front door is a screen of the section rather than the way out.
  assert.equal(firstScreen(INSTANCE), '/instance');
});

test('the private hubs row is drawn only where a private hub exists (UC-ID-16 check 3)', () => {
  const rowsOf = (groups: typeof ADMINISTRATION) => groups.flatMap((group) => group.rows).map((row) => row.id);
  assert.ok(rowsOf(administration({ hasPrivateHubs: true })).includes('private-hubs'));
  assert.ok(!rowsOf(administration({ hasPrivateHubs: false })).includes('private-hubs'));
  const allOn = { modulesOn: ['WORKFLOWS', 'ASSETS', 'TIME', 'AGENTS', 'SERVICE_DESK'] };
  assert.equal(rowsOf(administration({ hasPrivateHubs: false, ...allOn })).length, rowsOf(ADMINISTRATION).length - 1, 'nothing else goes');
});

test('on a single-workspace installation the installation is the administration\'s last group', () => {
  // UC-INS-04's story: on a private installation the one workspace is the installation, so its
  // owner finds the level above it at the end of Administration rather than in the menu - and an
  // operator who cannot reach the administration keeps the menu row, so it never disappears.
  const place = (single: boolean, admin: boolean, operator: boolean) =>
    installationPlace({ isSingleWorkspace: single, isAdministrationReachable: admin, isInstanceReachable: operator });
  assert.equal(place(true, true, true), 'administration');
  assert.equal(place(true, false, true), 'menu');
  assert.equal(place(false, true, true), 'menu');
  assert.equal(place(true, true, false), 'none');
  assert.equal(place(false, false, false), 'none');

  const menu = (single: boolean) =>
    account({ isAdministrationReachable: true, isInstanceReachable: true, isSingleWorkspace: single })
      .map((destination) => destination.id);
  assert.ok(!menu(true).includes('instance'), 'the menu row stayed beside the administration\'s group');
  assert.ok(menu(false).includes('instance'));

  const groups = administration({ hasPrivateHubs: true, withInstallation: true });
  assert.equal(groups.at(-1)?.id, 'installation', 'the installation is not the last group');
  assert.deepEqual(groups.at(-1)?.rows.map((row) => row.path), ['/instance']);
  assert.ok(!administration({ hasPrivateHubs: true }).some((group) => group.id === 'installation'));
});

// UC-AI-06 check 8: a single-workspace installation has no model to offer anybody, so its
// installation section has no *AI*; a multi-workspace one keeps it.
test('the installation offers no AI, platform, plans, usage or templates row where it runs one workspace', () => {
  const rowsOf = (single: boolean) => instanceSection({ isSingleWorkspace: single }).flatMap((group) => group.rows.map((row) => row.id));
  assert.ok(!rowsOf(true).includes('ai'), 'a private installation is offered an AI row');
  assert.ok(!rowsOf(true).includes('platform'), 'a private installation is offered a purchase platform');
  assert.ok(!rowsOf(true).includes('plans'), 'a private installation is offered plans');
  // Single mode offers no usage read (`admin.multi_mode_required`).
  assert.ok(!rowsOf(true).includes('usage'), 'a private installation is offered usage');
  // The installation's presets are a level a single installation does not have (UC-INS-18).
  assert.ok(!rowsOf(true).includes('templates'), 'a private installation is offered templates');
  assert.ok(['ai', 'platform', 'plans', 'usage', 'templates'].every((id) => rowsOf(false).includes(id)), 'a multi-workspace installation lost a row');
  assert.deepEqual(rowsOf(true), rowsOf(false).filter((id) => !['ai', 'platform', 'plans', 'usage', 'templates'].includes(id)), 'more than the five rows went');
});

test('the jumble is a place only for somebody it is drawn for', () => {
  assert.ok(primary({ hasJumble: true }).some((destination) => destination.id === 'jumble'));
  assert.deepEqual(primary({ hasJumble: false }).map((destination) => destination.id), ['workspace', 'search']);
});

// user-experience.md §2.7: the navigation's words are plain nouns, translated in every catalogue;
// a word the same in English and German is a word nobody translated, unless it is a brand or a
// loanword the German reader uses as it is.
test('every navigation word is translated, but for a short list of words used as they are', () => {
  const SAME = new Set(['Hubtask', 'Hubs', 'Installation', 'Whiteboards', 'Workflows', 'Webhooks', 'Tokens']);
  const codes = [
    ...DESTINATIONS.map((destination) => destination.code),
    ...PRODUCTS.map((row) => row.code),
    ...KEEPING.map((row) => row.code),
    ...workspaceBand({ isAdministrationReachable: true }).map((row) => row.code),
    ...ADMINISTRATION.flatMap((group) => [...(group.code ? [group.code] : []), ...group.rows.map((row) => row.code)]),
    MORE_CODE,
    'app.nav.products',
    'app.nav.workspace_band',
  ];
  const german = GERMAN as Record<string, string>;
  const untranslated = [...new Set(codes)].filter((code) => SOURCE[code] === german[code] && !SAME.has(SOURCE[code] as string));
  assert.deepEqual(untranslated, [], `the same in both: ${untranslated.map((code) => `${code}=${SOURCE[code]}`).join(', ')}`);
});

// user-experience.md §8.2: the advanced rows fold under *More* while the workspace holds one person,
// a row already used stays out, a group folds two rows or none, and Templates stays where a
// newcomer starts.
test('the advanced rows fold while the workspace holds one person, and never what is used', () => {
  const folded = (groups: readonly SectionGroup[]) =>
    groups.flatMap((group) => group.rows.filter((row) => row.isFolded).map((row) => row.id));
  const base = {
    isPrivateHubsReadable: true, hasAnyPrivateHub: false, hasSeveralPeople: false,
    isInstanceReachable: false, isAdministrationReachable: true, isSingleWorkspace: false,
  };
  assert.deepEqual(folded(administrationFor({ ...base, isOnePerson: true })).sort(),
    ['link-types', 'runs', 'voice', 'webhooks']);
  // Webhooks in use: voice would be alone under *More*, and a group folds two rows or none.
  assert.deepEqual(folded(administrationFor({ ...base, isOnePerson: true, inUse: ['webhooks'] })).sort(),
    ['link-types', 'runs']);
  // Unknown is not one person: nothing folds while the workspace is still being read.
  assert.deepEqual(folded(administrationFor(base)), []);
  assert.deepEqual(folded(administrationFor({ ...base, hasSeveralPeople: true, isOnePerson: false })), []);
  // Folded rows are still rows, so the palette, which reads the same list, finds them.
  const ids = administrationFor({ ...base, isOnePerson: true }).flatMap((group) => group.rows.map((row) => row.id));
  assert.ok(ids.includes('voice') && ids.includes('presets'));
});

test('the Android app keeps the administration\'s and the installation\'s rows, and leaves them to the web app (UC-SYN-07 check 7)', () => {
  const android = ['administration', 'instance'] as const;
  // The administration's row stands in the column's workspace band (user-experience.md §2.1), the
  // installation's in the account menu.
  const rows = account({ isAdministrationReachable: true, isInstanceReachable: true, holdsWorkspaces: true });
  const band = workspaceBand({ isAdministrationReachable: true });
  assert.deepEqual(
    rows.map((row) => row.id).slice(0, 4),
    ['profile', 'appearance', 'instance', 'switch-workspace'],
    'the rows stay where they are',
  );
  assert.deepEqual(band.map((row) => row.id), ['administration'], 'the column keeps the administration\'s row');
  assert.deepEqual(
    [...band, ...rows].map((row) => leftToWebApp(row.id, android)).filter(Boolean),
    ['administration', 'instance'],
  );
  assert.equal(leftToWebApp('administration', undefined), undefined, 'the browser and the desktop draw the area themselves');
  assert.equal(leftToWebApp('profile', android), undefined, 'the profile is the phone\'s own');
  // Somebody who does not reach the area is not offered its row on the phone either (P-05).
  const member = [...workspaceBand({ isAdministrationReachable: false }), ...account({ isAdministrationReachable: false, isInstanceReachable: false, holdsWorkspaces: true })];
  assert.equal(member.some((row) => leftToWebApp(row.id, android) !== undefined), false);
});

// ---------------------------------------------------------------------------------------------------
// The sidebar's two levels (the navigation concept, UC-WRK-36, UC-INS-21 check 11)
// ---------------------------------------------------------------------------------------------------

const LEVEL_HUBS: Record<string, string> = { h: 'h', c: 'h' };
const level = (name: string, params: Record<string, string> = {}, query: Record<string, string> = {}, located?: string) =>
  levelOf({ name, params, query }, (each) => LEVEL_HUBS[each], located);

test('anything of a hub shows its hub\'s level; everything else the workspace level (§2.4, §6.1)', () => {
  for (const name of ['hub', 'hub-plan', 'hub-plan-releases', 'hub-plan-sprints', 'hub-desk', 'hub-inventory', 'hub-inventory-section', 'hub-time', 'hub-time-workload', 'hub-time-budget', 'hub-goals', 'hub-goals-dashboards', 'hub-pages', 'hub-contents', 'hub-settings']) {
    assert.deepEqual(level(name, { id: 'h' }), { kind: 'hub', hubId: 'h' }, name);
  }
  for (const name of ['collection', 'collection-timeline', 'collection-sprint', 'collection-time', 'collection-workload', 'collection-settings']) {
    assert.deepEqual(level(name, { id: 'c' }), { kind: 'hub', hubId: 'h' }, name);
  }
  assert.deepEqual(level('item', { id: 'i' }, {}, 'c'), { kind: 'hub', hubId: 'h' }, 'an entry shows its collection\'s hub');
  assert.deepEqual(level('page', { id: 'p' }, {}, 'h'), { kind: 'hub', hubId: 'h' }, 'a page shows its space');
  assert.deepEqual(level('item', { id: 'i' }), { kind: 'workspace' }, 'until the entry says where it lives');
  for (const name of ['home', 'search', 'jumble', 'plans-home', 'pages-home', 'goals', 'trash', 'archive']) {
    assert.deepEqual(level(name), { kind: 'workspace' }, name);
  }
});

test('For you is Today, the inbox and what waits for a yes - never Search or Capture (§2.2)', () => {
  assert.deepEqual(forYou({ hasJumble: true, modulesOn: ['SERVICE_DESK'] }).map((row) => row.id), ['workspace', 'jumble', 'approvals']);
  assert.deepEqual(forYou({ hasJumble: false, modulesOn: [] }).map((row) => row.id), ['workspace']);
  // On compact the bottom bar carries Today and Inbox; the drawer keeps what it does not.
  assert.deepEqual(forYou({ hasJumble: true, modulesOn: ['SERVICE_DESK'], besideBar: true }).map((row) => row.id), ['approvals']);
});

test('the foot is Administration where reached, then Archive and Trash (§2.5)', () => {
  assert.deepEqual(foot({ isAdministrationReachable: true }).map((row) => row.id), ['administration', 'archive', 'trash']);
  assert.deepEqual(foot({ isAdministrationReachable: false }).map((row) => row.id), ['archive', 'trash']);
});

test('the hub tools stand in one fixed order, each only where its module is on, Queues only with a desk (UC-INS-21 check 11)', () => {
  const all = ['SERVICE_DESK', 'PLANNING', 'ASSETS', 'TIME', 'GOALS', 'KNOWLEDGE', 'AGENTS'];
  assert.deepEqual(hubTools({ modulesOn: all, hasDesk: true }).map((tool) => tool.id), ['queues', 'plan', 'inventory', 'time', 'goals']);
  assert.deepEqual(hubTools({ modulesOn: all, hasDesk: false }).map((tool) => tool.id), ['plan', 'inventory', 'time', 'goals']);
  for (const tool of HUB_TOOLS) {
    const without = hubTools({ modulesOn: all.filter((each) => each !== tool.module), hasDesk: true }).map((each) => each.id);
    assert.ok(!without.includes(tool.id), `${tool.module} off keeps ${tool.id}`);
    assert.equal(without.length, 4, `${tool.module} off takes more than its tool`);
  }
  assert.deepEqual(hubTools({ modulesOn: [], hasDesk: true }), [], 'a household sees no tool (D1)');
});

test('every tool opens a route that names it current, and the switch keeps the tool where the target has it', () => {
  const routeOf: Record<string, LevelRoute> = {
    '/hubs/h/desk': { name: 'hub-desk', params: {}, query: {} },
    '/hubs/h/plan': { name: 'hub-plan', params: {}, query: {} },
    '/hubs/h/inventory': { name: 'hub-inventory', params: {}, query: {} },
    '/hubs/h/time': { name: 'hub-time', params: {}, query: {} },
    '/hubs/h/goals': { name: 'hub-goals', params: {}, query: {} },
  };
  for (const tool of HUB_TOOLS) assert.equal(toolOf(routeOf[toolPath('h', tool.id)]!), tool.id, tool.id);
  assert.equal(toolOf({ name: 'hub', query: {} }), 'overview');
  assert.equal(toolOf({ name: 'goals', query: {} }), undefined, 'the workspace\'s goals are no hub tool');
  const planOnly = hubTools({ modulesOn: ['PLANNING'], hasDesk: false });
  assert.equal(switchPath('x', 'plan', planOnly), '/hubs/x/plan');
  assert.equal(switchPath('x', 'time', planOnly), '/hubs/x');
  assert.equal(switchPath('x', 'overview', planOnly), '/hubs/x');
});

test('the products follow the person\'s order; hidden ones go behind More; one switched off is gone (§2.3)', () => {
  const on = ['PLANNING', 'KNOWLEDGE', 'TIME'];
  assert.deepEqual(navProducts({ modulesOn: on }).shown.map((row) => row.module), ['PLANNING', 'KNOWLEDGE', 'TIME']);
  const sets = navProducts({ modulesOn: on, order: ['TIME', 'PLANNING'], hidden: ['KNOWLEDGE', 'ASSETS'] });
  assert.deepEqual(sets.shown.map((row) => row.module), ['TIME', 'PLANNING']);
  assert.deepEqual(sets.more.map((row) => row.module), ['KNOWLEDGE']);
  assert.deepEqual(navProducts({ modulesOn: [] }), { shown: [], more: [] });
});

test('the hubs in the shared order, recently visited first, or A-Z (§2.3)', () => {
  const hubs = [{ id: 'b', name: 'Beta' }, { id: 'a', name: 'alpha' }, { id: 'c', name: 'Gamma' }];
  assert.deepEqual(orderHubs(hubs, 'MANUAL').map((each) => each.id), ['b', 'a', 'c']);
  assert.deepEqual(orderHubs(hubs, null).map((each) => each.id), ['b', 'a', 'c']);
  assert.deepEqual(orderHubs(hubs, 'RECENT', ['c']).map((each) => each.id), ['c', 'b', 'a']);
  assert.deepEqual(orderHubs(hubs, 'NAME', [], 'en').map((each) => each.id), ['a', 'b', 'c']);
});

test('queues: the starred first, then the hub\'s order; a breaching one is urgent; a standard queue is named by its key', () => {
  const rows = queueRows(
    [
      { view_id: 'v1', key: 'mine', name: 'Mine', count: 3, at_risk: 0 },
      { view_id: 'v2', key: 'breach', name: 'About to breach', count: 1, at_risk: 1 },
      { view_id: null, key: 'open', name: null, count: null, at_risk: null },
    ],
    ['v2', 'gone'],
  );
  assert.deepEqual(rows.map((row) => row.id), ['v2', 'v1', 'std:open']);
  assert.deepEqual(rows.map((row) => row.isUrgent), [true, false, false]);
  assert.equal(queuePath('h', rows[2]!), '/hubs/h/desk?queue=open');
});

test('the content tree: pages and boards by their shared key, keyless boards after A-Z, no templates, no child under a board', () => {
  const pages = [
    { id: 'p1', parent_id: null, title: 'Start', order_key: 'a1' },
    { id: 'p2', parent_id: null, title: 'Team', order_key: 'a3' },
    { id: 'p3', parent_id: 'p2', title: 'Onboarding', order_key: 'a1' },
    { id: 't', parent_id: null, title: 'A template', order_key: 'a0', template: true },
  ];
  const boards = [
    { id: 'b1', title: 'Retro', parent_page_id: 'p2', order_key: 'a2' },
    { id: 'b2', title: 'Map', parent_page_id: null, order_key: 'a2' },
    { id: 'b3', title: 'Zoo', parent_page_id: null, order_key: null },
    { id: 'b4', title: 'Atlas', parent_page_id: null, order_key: null },
    { id: 'b5', title: 'Hidden parent', parent_page_id: 'unseen', order_key: 'a9' },
  ];
  const tree = contentsTree(pages, boards);
  assert.deepEqual(tree.map((node) => node.id), ['p1', 'b2', 'p2', 'b5', 'b4', 'b3']);
  assert.deepEqual(tree[2]!.children.map((node) => node.id), ['p3', 'b1']);
  assert.ok(tree.filter((node) => node.kind === 'whiteboard').every((node) => node.children.length === 0));
  assert.equal(contentsCount(tree), 8);
  assert.deepEqual(contentsTree(pages, boards, 'NAME', 'en').map((node) => node.title), ['Atlas', 'Hidden parent', 'Map', 'Onboarding', 'Retro', 'Start', 'Team', 'Zoo']);
  const recent = contentsTree([{ id: 'x', parent_id: null, title: 'Old', updated_at: '2026-01-01' }], [{ id: 'y', title: 'New', updated_at: '2026-02-01' }], 'RECENT');
  assert.deepEqual(recent.map((node) => node.id), ['y', 'x']);
});

test('a row placed after a sibling lands at that index among the others', () => {
  assert.equal(placedIndex(['a', 'b', 'c'], 'c', null), 0);
  assert.equal(placedIndex(['a', 'b', 'c'], 'a', 'c'), 2);
  assert.equal(placedIndex(['a', 'b', 'c'], 'a', 'b'), 1);
});

test('a For you count never says 0, and says n+ where its read goes on', () => {
  assert.equal(figureCount(0), undefined);
  assert.equal(figureCount(5), '5');
  assert.equal(figureCount(50, true), '50+');
});
