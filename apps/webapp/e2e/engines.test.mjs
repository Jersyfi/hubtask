// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The browser job (ADR-0048): the built bundle, loaded in Chromium, Firefox and WebKit, and
// ADR-0044's feature table asserted in each - not a journey. Every assertion is a fact about the
// engine that fails loudly in one that lacks the feature: a modal dialog traps focus and makes
// the page behind it inert, the focus ring lands where design-system.md's rule 5 puts it, a
// visually-hidden label is in the tree and not on the screen, an overlay is placed by CSS anchor
// positioning. What runs is `dist/`, served the way the binary serves it (serve.mjs).
//
// The API is stubbed at the network edge, once, with the three answers the frame needs to draw a
// signed-in workspace - an account, one hub, and a manifest saying the account may shape the
// workspace (so *Create hub* is drawn) - and nothing else: the stream is refused, every
// other call answers an empty page. That is the least a workspace can be, and it is enough for a
// dialog, a focus ring and a hidden label; deeper screens have walks of their own.
//
// `node --test e2e/` rather than the package's `test` script: this needs the three browsers
// installed (`pnpm exec playwright install --with-deps`), and the unit tests must keep running
// where they are not.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium, firefox, webkit } from 'playwright';

import { COLLECTION, ITEMS, stub as fixtureStub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const ACCOUNT = {
  id: '01a0e2e0-0000-7000-8000-000000000001',
  kind: 'USER',
  display_name: 'Engine Walker',
  email: 'engines@example.invalid',
  status: 'ACTIVE',
  locale: 'en',
  // Taken already, so that the walk below is not led through the tour; the tour has a
  // walk of its own at the end, as an account that never took it.
  onboarding_completed_at: '2026-09-01T00:00:00Z',
};
const FRESH_ACCOUNT = { ...ACCOUNT, id: '01a0e2e0-0000-7000-8000-000000000009', onboarding_completed_at: null };
const HUB = {
  id: '01a0e2e0-0000-7000-8000-000000000002',
  type: 'HUB',
  parent_id: null,
  name: 'Engines',
  order_key: 'a0',
  version: 1,
};
const EMPTY_PAGE = { data: [], items: [], page: { next_cursor: null, has_more: false } };

/** The API at the network edge: an account, a hub, and empty pages for everything else. */
async function stub(route) {
  const url = new URL(route.request().url());
  if (url.pathname.endsWith('/api/v1/stream')) return route.abort();
  // The initial synchronisation: the hub, and the cursor line that ends it.
  if (url.pathname.endsWith('/api/v1/sync:snapshot')) {
    const record = JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: HUB.id, container_id: null, payload: HUB });
    return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${record}\n{"cursor":"c-e2e"}\n` });
  }
  if (url.pathname.endsWith('/api/v1/sync:pull')) {
    return route.fulfill({ json: { changes: [], cursor: 'c-e2e', has_more: false, tombstone_window_days: 90 } });
  }
  if (url.pathname.endsWith('/api/v1/accounts/me')) return route.fulfill({ json: ACCOUNT });
  if (url.pathname.endsWith('/api/v1/meta/capabilities')) {
    return route.fulfill({ json: { ...EMPTY_PAGE, workspace_permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE'], private_hub_offered: true } });
  }
  if (url.pathname.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') {
    return route.fulfill({ json: { ...EMPTY_PAGE, data: [HUB] } });
  }
  // A bare list, as the contract answers it - not a page: the create dialog reads it.
  if (url.pathname.endsWith('/api/v1/presets')) return route.fulfill({ json: [] });
  return route.fulfill({ json: EMPTY_PAGE });
}

/** ADR-0044's feature table, asked of the engine itself. */
const FEATURES = {
  'dialog.showModal': () => typeof HTMLDialogElement?.prototype?.showModal === 'function',
  inert: () => 'inert' in HTMLElement.prototype,
  ':has()': () => CSS.supports('selector(:has(a))'),
  popover: () => typeof HTMLElement.prototype.showPopover === 'function',
  'clip-path': () => CSS.supports('clip-path: inset(50%)'),
  'logical properties': () => CSS.supports('inset-inline-start: 0'),
  'anchor-name': () => CSS.supports('anchor-name: --hbt'),
  'position-area': () => CSS.supports('position-area: block-start'),
  'position-try-fallbacks': () => CSS.supports('position-try-fallbacks: flip-block'),
};

// CI runs Chromium only (ADR-0089); ENGINES=firefox,webkit runs the others by hand.
const ENGINES = Object.fromEntries(
  Object.entries({ chromium, firefox, webkit }).filter(
    ([name]) => !process.env.ENGINES || process.env.ENGINES.split(',').includes(name),
  ),
);

const served = await serve(DIST);
test.after(() => served.close());

for (const [name, engine] of Object.entries(ENGINES)) {
  test(`${name}: the client runs, and the engine has what it is built on`, async (t) => {
    const browser = await engine.launch();
    t.after(() => browser.close());
    const context = await browser.newContext();
    await context.route('**/api/v1/**', stub);
    // A session the frame believes: the pair lives in sessionStorage (platform/browser.ts).
    await context.addInitScript(() => {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    });
    const page = await context.newPage();
    const failures = [];
    page.on('pageerror', (error) => failures.push(String(error)));

    await page.goto(`${served.origin}/`);
    // *Today*, drawn: the page the app opens on, past the session and the manifest.
    await page.getByRole('region', { name: 'On you' }).waitFor({ state: 'visible', timeout: 15_000 });
    // The navigation's way to make a hub: the dialog this walk opens below.
    const createHub = page.getByRole('button', { name: 'Create hub', exact: true }).first();
    await createHub.waitFor({ state: 'visible', timeout: 15_000 });
    assert.deepEqual(failures, [], `${name}: the bundle threw while booting`);

    // The table, engine by engine: each of these is one thing the client is built on
    // (ADR-0044), and a `false` here is the engine saying so before any screen could.
    for (const [feature, probe] of Object.entries(FEATURES)) {
      assert.equal(await page.evaluate(probe), true, `${name} lacks ${feature}`);
    }

    // A visually-hidden label is in the accessibility tree and not on the screen: the skip link,
    // until it takes focus. One pixel, clipped - never `display: none`.
    const skip = page.getByRole('link', { name: 'Skip to the content' });
    assert.equal(await skip.count(), 1, `${name}: the skip link is not in the tree`);
    const hiddenBox = await skip.evaluate((el) => {
      const wrapper = el.closest('.visually-hidden');
      const rect = wrapper.getBoundingClientRect();
      return { w: rect.width, h: rect.height, clip: getComputedStyle(wrapper).clipPath };
    });
    assert.deepEqual([hiddenBox.w, hiddenBox.h], [1, 1], `${name}: the hidden wrapper is ${JSON.stringify(hiddenBox)}`);
    assert.equal(hiddenBox.clip, 'inset(50%)', `${name}: the clip pattern is ${hiddenBox.clip}`);

    // The focus ring lands where rule 5 puts it: the first keyboard step reaches the skip link,
    // which reveals itself, and the ring is the token's outline rather than the engine's default.
    // WebKit keeps links out of the Tab order, as Safari does by default; there the step that
    // reaches a link is Option+Tab, and the client cannot and should not change that.
    await page.keyboard.press('Tab');
    if (await page.evaluate(() => document.activeElement?.tagName !== 'A')) {
      await page.keyboard.press('Shift+Tab');
      await page.keyboard.press('Alt+Tab');
    }
    const ring = await page.evaluate(() => {
      const el = document.activeElement;
      const style = getComputedStyle(el);
      const wrapper = el.closest('.visually-hidden');
      return {
        name: el.textContent.trim(),
        outlineStyle: style.outlineStyle,
        outlineWidth: Number.parseFloat(style.outlineWidth),
        revealed: wrapper ? wrapper.getBoundingClientRect().width > 1 : null,
      };
    });
    assert.equal(ring.name, 'Skip to the content', `${name}: the first keyboard step landed on ${ring.name}`);
    assert.equal(ring.revealed, true, `${name}: the skip link did not reveal itself on focus`);
    assert.notEqual(ring.outlineStyle, 'none', `${name}: no focus ring`);
    assert.ok(ring.outlineWidth > 0, `${name}: the focus ring is ${ring.outlineWidth}px wide`);

    // A dialog opens as a modal, takes focus, traps it, and makes the page behind it inert - so a
    // control behind it is unreachable by keyboard, which is what a gated control has to be.
    // Opened from the keyboard, because that is the case focus return is for: WebKit, like
    // Safari, does not focus a button a pointer clicks, so a pointer's dialog has no opener to
    // return to and the engine leaves focus on the body - as Safari does.
    await createHub.focus();
    await page.keyboard.press('Enter');
    const dialog = page.locator('dialog[open]');
    await dialog.waitFor({ state: 'visible', timeout: 5_000 });
    assert.equal(await dialog.getAttribute('aria-labelledby') !== null, true, `${name}: the dialog has no name`);
    assert.equal(await page.evaluate(() => document.activeElement?.closest('dialog[open]') !== null), true,
      `${name}: focus did not enter the dialog`);
    // Twelve Tabs is more than the dialog has controls: focus stays in it, or sits on the body
    // for the one step in which an engine hands it to its own chrome on the way round - what
    // must never happen is a control behind the dialog taking it.
    for (let i = 0; i < 12; i++) {
      await page.keyboard.press('Tab');
      const landed = await page.evaluate(() => {
        const el = document.activeElement;
        if (!el || el === document.body) return 'body';
        return el.closest('dialog[open]') ? 'dialog' : el.outerHTML.slice(0, 80);
      });
      assert.ok(landed === 'dialog' || landed === 'body', `${name}: Tab ${i + 1} left the dialog for ${landed}`);
    }
    const reachedBehind = await page.evaluate(() => {
      const behind = document.querySelector('nav a, nav button');
      behind?.focus();
      return document.activeElement === behind;
    });
    assert.equal(reachedBehind, false, `${name}: a control behind the modal took focus`);

    // Where the Tabs ended on a control that cannot be used yet, its reason is showing, and the
    // first `Escape` is that bubble's (WCAG 1.4.13; design-system.md §4): one more closes the dialog.
    const reasonShowing = () => page.evaluate(() =>
      (document.activeElement?.getAttribute('aria-describedby') ?? '').split(' ')
        .map((id) => document.getElementById(id))
        .some((node) => node?.getAttribute('role') === 'tooltip' && !node.hidden));
    if (await reasonShowing()) {
      await page.keyboard.press('Escape');
      assert.equal(await reasonShowing(), false, `${name}: Escape left the reason showing`);
    }
    await page.keyboard.press('Escape');
    await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
    // Back on the trigger - polled, because an engine hands focus back a task after `close()`.
    await page.waitForFunction(() => document.activeElement?.textContent?.trim() === 'Create hub', null, { timeout: 5_000 })
      .catch(async () => {
        const landed = await page.evaluate(() => document.activeElement?.outerHTML.slice(0, 80));
        assert.fail(`${name}: focus did not return to the trigger but sits on ${landed}`);
      });

    // The replica: the engine opened this account's database in this engine's IndexedDB
    // and kept the snapshot's cursor in it - one database per origin and account, and gone whole
    // at sign-out (offline-sync.md §9.6). Asked of the engine itself, because a fake IndexedDB
    // in Node proves the store's logic and not the engine's behaviour.
    const database = `hubtask:${served.origin}:${ACCOUNT.id}`;
    await page.waitForFunction(async (name) => (await indexedDB.databases()).some((d) => d.name === name), database, { timeout: 10_000 })
      .catch(() => assert.fail(`${name}: the replica's database was not opened`));
    const position = await page.evaluate((name) => new Promise((resolve, reject) => {
      const open = indexedDB.open(name);
      open.onerror = () => reject(open.error);
      open.onsuccess = () => {
        const db = open.result;
        const get = db.transaction('records', 'readonly').objectStore('records').get(['meta', 'position']);
        get.onsuccess = () => { db.close(); resolve(get.result?.value ?? null); };
        get.onerror = () => { db.close(); reject(get.error); };
      };
    }), database);
    assert.equal(position?.cursor, 'c-e2e', `${name}: the cursor in the store is ${JSON.stringify(position)}`);

    // Reads answered by the replica: the server goes away, the tab reloads, and the tree
    // is drawn from the copy with the one line that says so - as of the store's last
    // synchronisation. What the copy does not hold is named `sync.needs_connection`.
    await context.unroute('**/api/v1/**');
    await context.route('**/api/v1/**', (route) => route.abort('connectionfailed'));
    // Counted while the server is away: a read that fails is read again on the reconnect and
    // not before: a read retried on every failure is a loop as tight as the network lets it be.
    let accountReads = 0;
    const countAccountReads = (request) => { if (new URL(request.url()).pathname.endsWith('/accounts/me')) accountReads += 1; };
    page.on('request', countAccountReads);
    await page.reload();
    const mark = page.getByRole('status').filter({ hasText: "Shown from this device's copy" });
    await mark.first().waitFor({ state: 'visible', timeout: 15_000 })
      .catch(() => assert.fail(`${name}: the tree was not drawn from the replica while the server was away`));
    // The tree, and the tree alone: the overview stopped listing the hubs when it became what is
    // on the reader (ADR-0063 decision 1), so a hub drawn from the copy is a row of the navigation
    // and nothing else on the page draws it a second time.
    assert.equal(await page.getByRole('treeitem', { name: 'Engines' }).count(), 1, `${name}: the hub in the copy is not in the tree`);

    // The server comes back, and the copy's state is replaced by the server's - on the loop's
    // reconnect, which is what the pause is for: long enough for the first attempt to have
    // failed, so the replacement is the retry's and not the first attempt's luck.
    await page.waitForTimeout(1_500);
    page.off('request', countAccountReads);
    assert.ok(accountReads <= 4, `${name}: the account was read ${accountReads} times while the server was away`);
    await context.unroute('**/api/v1/**');
    await context.route('**/api/v1/**', stub);
    await page.waitForFunction(() => ![...document.querySelectorAll('[role=status]')].some((el) => el.textContent?.includes("Shown from this device's copy")), null, { timeout: 30_000 })
      .catch(() => assert.fail(`${name}: the replica's state was not replaced after the server came back`));

    // The account, which the copy does not hold, comes back with the server too:
    // the engine reads again what it could not answer, so the menu says the name and not "You".
    await page.getByRole('button', { name: 'Engine Walker' }).waitFor({ timeout: 15_000 })
      .catch(() => assert.fail(`${name}: the account was not read again after the server came back`));

    // Sign-out deletes the database, not its rows. The verb is the last item of the account
    // menu, behind the name (ADR-0061 decision 1).
    await page.getByRole('button', { name: 'Engine Walker' }).click();
    await page.getByRole('menuitem', { name: 'Sign out' }).click();
    await page.waitForFunction(async (name) => !(await indexedDB.databases()).some((d) => d.name === name), database, { timeout: 10_000 })
      .catch(() => assert.fail(`${name}: the replica's database survived the sign-out`));
    assert.deepEqual(failures, [], `${name}: the bundle threw during the walk`);

    // The tour: an account that never took it is led through on arrival. The spotlight's
    // cut-out is positioned by CSS - anchor positioning gives it the element's box (ADR-0039) -
    // so the engine, not a measurement, decides where it is: the computed `position-anchor`
    // names the element's anchor, no inline offset is written, and the box is the element's with
    // the air the stylesheet adds. Escape skips, and skipping writes `onboarding_completed_at`.
    const fresh = await browser.newContext();
    const written = [];
    await fresh.route('**/api/v1/**', async (route) => {
      const url = new URL(route.request().url());
      if (url.pathname.endsWith('/api/v1/accounts/me')) return route.fulfill({ json: FRESH_ACCOUNT });
      if (url.pathname.endsWith('/preferences') && route.request().method() === 'PATCH') {
        written.push(route.request().postDataJSON());
        return route.fulfill({ json: { ...FRESH_ACCOUNT, ...route.request().postDataJSON() } });
      }
      return stub(route);
    });
    await fresh.addInitScript(() => {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    });
    const guided = await fresh.newPage();
    guided.on('pageerror', (error) => failures.push(String(error)));
    await guided.goto(`${served.origin}/`);
    const spotlight = guided.locator('.spotlight');
    await spotlight.waitFor({ state: 'attached', timeout: 15_000 })
      .catch(() => assert.fail(`${name}: the tour did not start for an account that never took it`));
    const cutout = await spotlight.evaluate((el) => {
      const target = document.querySelector('[data-hbt-spotlit]');
      const style = getComputedStyle(el);
      const box = (node) => { const r = node.getBoundingClientRect(); return [r.left, r.top, r.width, r.height].map(Math.round); };
      return {
        anchor: style.positionAnchor,
        inline: el.getAttribute('style') ?? '',
        popover: el.getAttribute('popover'),
        spot: box(el),
        target: target ? box(target) : null,
        tour: target?.getAttribute('data-tour'),
      };
    });
    assert.match(cutout.anchor, /^--hbt-spot-/, `${name}: the cut-out's position-anchor is ${JSON.stringify(cutout.anchor)}`);
    assert.equal(cutout.inline, '', `${name}: the cut-out carries an inline offset: ${cutout.inline}`);
    assert.equal(cutout.popover, 'manual', `${name}: the cut-out is not in the top layer`);
    assert.equal(cutout.tour, 'hubs', `${name}: the first step points at ${cutout.tour}`);
    // The cut-out is the element's box grown by the air around it: within it on every side,
    // and by less than the largest spacing step.
    assert.ok(cutout.target && cutout.spot[0] <= cutout.target[0] && cutout.spot[1] <= cutout.target[1]
      && cutout.spot[0] + cutout.spot[2] >= cutout.target[0] + cutout.target[2]
      && cutout.spot[1] + cutout.spot[3] >= cutout.target[1] + cutout.target[3]
      && cutout.target[0] - cutout.spot[0] < 32 && cutout.target[1] - cutout.spot[1] < 32,
      `${name}: the cut-out ${JSON.stringify(cutout.spot)} does not sit on the element ${JSON.stringify(cutout.target)}`);
    const coachMark = guided.getByRole('dialog', { name: 'Where work lives' });
    await coachMark.waitFor({ state: 'visible', timeout: 5_000 }).catch(() => assert.fail(`${name}: the coach mark is not a named dialog`));
    assert.equal(await guided.evaluate(() => document.activeElement?.textContent?.trim()), 'Next', `${name}: focus did not move to the mark`);
    await guided.keyboard.press('Escape');
    await spotlight.waitFor({ state: 'detached', timeout: 5_000 }).catch(() => assert.fail(`${name}: Escape did not skip the tour`));
    await guided.waitForTimeout(500);
    assert.ok(written.some((body) => typeof body?.onboarding_completed_at === 'string'), `${name}: skipping did not write onboarding_completed_at: ${JSON.stringify(written)}`);
    await fresh.close();
    assert.deepEqual(failures, [], `${name}: the bundle threw during the tour`);
  });

  // UC-WRK-27 check 5 by keyboard alone, in every engine: the shortcut, the arrows and Enter
  // perform the highlighted row, and Escape closes and returns focus where it was. Against the
  // shared fixture, which has the collection the palette is asked for.
  test(`${name}: the palette by keyboard alone`, async (t) => {
    const browser = await engine.launch();
    t.after(() => browser.close());
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    await context.route('**/api/v1/**', fixtureStub);
    await context.addInitScript(() => {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    });
    const page = await context.newPage();
    const failures = [];
    page.on('pageerror', (error) => failures.push(String(error)));

    await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
    const checkbox = page.getByRole('checkbox', { name: `Mark ${ITEMS[2].title} as done` });
    await checkbox.waitFor({ timeout: 15_000 });
    await page.waitForTimeout(600);
    await checkbox.focus();

    await page.keyboard.press('Control+k');
    const field = page.getByRole('combobox', { name: 'Search everything' });
    await page.locator('#search-menu').waitFor({ timeout: 5_000 });
    await page.keyboard.type('kitch');
    await page.getByRole('option', { name: /Open Kitchen as a board/ }).waitFor({ timeout: 5_000 });
    let found = false;
    for (let presses = 0; presses < 40 && !found; presses += 1) {
      await page.keyboard.press('ArrowDown');
      const active = await field.getAttribute('aria-activedescendant');
      found = active !== null && (await page.locator(`[id="${active}"] .title`).textContent())?.trim() === 'Open Kitchen as a board';
    }
    assert.ok(found, `${name}: the arrows never reached the row`);
    await page.keyboard.press('Enter');
    await page.getByRole('radio', { name: 'Board', checked: true }).waitFor({ timeout: 5_000 });

    const board = page.getByRole('radio', { name: 'Board' });
    await board.focus();
    await page.keyboard.press('Control+k');
    await page.locator('#search-menu').waitFor({ timeout: 5_000 });
    await page.keyboard.press('Escape');
    await page.locator('#search-menu').waitFor({ state: 'detached', timeout: 5_000 });
    assert.ok(await board.evaluate((node) => node === document.activeElement), `${name}: Escape did not return focus`);
    await context.close();
    assert.deepEqual(failures, [], `${name}: the bundle threw in the palette`);
  });

  // UC-WRK-26 check 6 by keyboard alone, in every engine: `Q` opens the field, a chip is taken
  // back from the keyboard and focus comes back to the field, Enter captures, Escape returns focus.
  test(`${name}: the capture field by keyboard alone`, async (t) => {
    const browser = await engine.launch();
    t.after(() => browser.close());
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    await context.route('**/api/v1/**', fixtureStub);
    await context.addInitScript(() => {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    });
    const page = await context.newPage();
    const failures = [];
    page.on('pageerror', (error) => failures.push(String(error)));
    const creates = [];
    page.on('request', (request) => {
      if (request.method() === 'POST' && new URL(request.url()).pathname.endsWith('/api/v1/items')) creates.push(request.postDataJSON());
    });

    await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
    const checkbox = page.getByRole('checkbox', { name: `Mark ${ITEMS[2].title} as done` });
    await checkbox.waitFor({ timeout: 15_000 });
    await page.waitForTimeout(600);
    await checkbox.focus();

    await page.keyboard.press('q');
    const field = page.getByRole('textbox', { name: 'What do you want to remember?' });
    await field.waitFor({ timeout: 5_000 });
    await page.keyboard.type('Order tiles !Materials');
    await page.getByRole('button', { name: 'Keep “!Materials” in the title' }).waitFor({ timeout: 5_000 });
    // WebKit's Tab reaches text fields only, as Safari's default does; Option+Tab is its way to a
    // button, and the reader on that engine uses the same.
    await page.keyboard.press(name === 'webkit' ? 'Alt+Tab' : 'Tab');
    await page.keyboard.press('Enter');
    assert.ok(await field.evaluate((node) => node === document.activeElement), `${name}: focus did not come back to the field`);
    await page.keyboard.press('Enter');
    for (let waited = 0; creates.length === 0 && waited < 50; waited++) await page.waitForTimeout(100);
    assert.equal(creates[0]?.title, 'Order tiles !Materials', `${name}: the capture was not sent as typed`);

    await checkbox.focus();
    await page.keyboard.press('q');
    await field.waitFor({ timeout: 5_000 });
    await page.keyboard.press('Escape');
    await field.waitFor({ state: 'hidden', timeout: 5_000 });
    assert.ok(await checkbox.evaluate((node) => node === document.activeElement), `${name}: Escape did not return focus`);
    await context.close();
    assert.deepEqual(failures, [], `${name}: the bundle threw in the capture field`);
  });
}

// UC-WRK-32 check 5 in every engine: a value set above carries the chip beside it, and pressing it
// opens the value, where it comes from and whom to ask - a popover, which each engine places itself.
for (const [name, engine] of Object.entries(ENGINES)) {
  test(`${name}: a limit set by the plan says so, and the chip opens its value, source and whom to ask`, async (t) => {
    const browser = await engine.launch();
    t.after(() => browser.close());
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    await context.route('**/api/v1/**', async (route) => {
      const url = new URL(route.request().url());
      if (url.pathname.endsWith('/api/v1/meta/capabilities')) {
        return route.fulfill({ json: { ...EMPTY_PAGE, tenancy_mode: 'multi', workspace_permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'READ_CONFIGURATION'] } });
      }
      if (url.pathname.endsWith('/api/v1/quotas')) {
        return route.fulfill({ json: [
          { quota: 'items', limit: 5000, used: 12, configured: false, locked: true, source: 'PLAN', lock: 'PLAN' },
        ] });
      }
      return stub(route);
    });
    await context.addInitScript(() => {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    });
    const page = await context.newPage();
    const failures = [];
    page.on('pageerror', (error) => failures.push(String(error)));

    await page.goto(`${served.origin}/administration/quotas`);
    const chip = page.getByRole('button', { name: 'Set by your plan, locked' });
    await chip.waitFor({ state: 'visible', timeout: 15_000 }).catch(() => assert.fail(`${name}: no chip beside the limit`));
    await chip.click();
    const opened = page.getByText('Set by your plan, and locked: it cannot be changed here.');
    await opened.waitFor({ state: 'visible', timeout: 5_000 }).catch(() => assert.fail(`${name}: the chip opened nothing`));
    assert.ok(await page.getByText('Ask your provider.').isVisible(), `${name}: the chip does not say whom to ask`);
    await page.keyboard.press('Escape');
    await opened.waitFor({ state: 'hidden', timeout: 5_000 }).catch(() => assert.fail(`${name}: Escape did not close the chip`));
    assert.deepEqual(failures, [], `${name}: the bundle threw on the limits`);
  });
}
