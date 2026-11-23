// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The entry screen when `/meta/capabilities` has not been read. Everything an entry
// shows below its title is the manifest's answer, and "not read yet" must not render exactly
// like "this type refuses it": no rows, no notes, no subtree, and not a word about why.
//
// Three roads to that screen are walked here, because they are three different defects:
//
//   1. the read failed and stayed failed — the column has to say so rather than be empty;
//   2. the reader has to be able to ask again from where they are, which is the mark the frame
//      keeps for what the application says about **itself** (ADR-0065 decision 4) and not a page
//      ADR-0063 decision 6 took out of the navigation. It moved there from the connection's mark:
//      that one is drawn only with a session, and the manifest is read before anybody signs in;
//   3. a stale bearer at boot signs the reader out through the manifest's own request, and the
//      sign-in that follows has to read it again — as that actor, because the server scopes the
//      answer by the caller.
//
// Chromium only, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { serve } from './serve.mjs';
import { ACCOUNT, ITEMS, MANIFEST, stub } from './fixture.mjs';
import { unfoldFields } from './reveal.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const ENTRY = ITEMS[0];
/** What a `TASK` holds, as the fixture's profile declares it. The column, when it can be read. */
const ROWS = ['assignee', 'due', 'labels', 'reminders', 'recurrence', 'language', 'cover', 'attachments'];

const served = await serve(DIST);
test.after(() => served.close());

const problem = (status, code) => ({
  status,
  contentType: 'application/problem+json',
  body: JSON.stringify({ type: 'about:blank', title: code, status, code }),
});

/**
 * The mark that carries what the application says about itself, and what it opens.
 *
 * `NoticeMark`, not the connection's `SyncStatus`: what is unread here is the installation rather
 * than a change of this copy's, and - the half that decides it - this mark is drawn on every
 * screen while the connection's needs a session (ADR-0065 decision 4).
 */
const markOf = (page) => page.getByRole('banner', { name: 'Application bar' }).locator('.notice-mark');

test('chromium: the manifest never answers — the entry says so, and the bar can ask again', async (t) => {
  const browser = await sharedChromium(t);

  // The read fails until the walk says otherwise, which is what the retry is measured against.
  // The engine reads it again on its own once its first synchronisation is answered, which a
  // loaded runner can bring to the moment of the press: a read that arrives while the walk
  // presses waits until the press is made, so that only *Try again* can be what brings it.
  let answers = false;
  let pressing;
  const context = await browser.newContext({ viewport: { width: 1280, height: 1000 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', async (route) => {
    if (new URL(route.request().url()).pathname.endsWith('/api/v1/meta/capabilities')) {
      await pressing;
      return answers ? route.fulfill({ json: MANIFEST }) : route.fulfill(problem(503, 'errors.service_unavailable'));
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

  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });

  // The entry's own values are there — they are the entry's, not the type's.
  assert.equal(await page.getByRole('heading', { level: 1, name: ENTRY.title }).count(), 1);

  // **Nothing is claimed about the type.** Not one row, and not the language row either: the
  // picker's own list of languages comes out of the same manifest, so a row offered here would be
  // a row with nothing in it.
  const drawn = await page.locator('[data-detail]').evaluateAll((rows) => rows.map((row) => row.getAttribute('data-detail')));
  assert.deepEqual(drawn, [], `the column drew ${JSON.stringify(drawn)} from a manifest nobody has`);

  // And it is said: a column simply empty reads exactly like a type that
  // carries nothing (voice-and-tone.md §4.4).
  const said = page.getByText('What this kind of entry can hold could not be read', { exact: false });
  await said.waitFor({ timeout: 5_000 }).catch(() => assert.fail('the column says nothing about why it is empty'));
  // The server's own sentence beside it, rather than this screen's paraphrase of a status code.
  // FailureNote draws it, beside the column's own sentence.
  const reason = page.locator('.unread + .banner[data-tone="danger"]');
  assert.equal(await reason.count(), 1, 'the reason the server gave is not shown');
  assert.notEqual((await reason.innerText()).trim(), '', 'the reason the server gave is empty');

  // The home link does not stand empty: without the manifest nobody knows whether this reader may
  // read `/tenant`, so the look every member may read is asked, and the product's mark is drawn (G8).
  const home = page.getByRole('banner', { name: 'Application bar' }).locator('a.home');
  await home.locator('.waiting').waitFor({ state: 'detached', timeout: 5_000 })
    .catch(() => assert.fail('the home link stays an empty box while the manifest is unread'));
  assert.equal(await home.locator('svg').first().isVisible(), true, 'the home link draws no mark');
  assert.equal(
    await page.getByRole('banner', { name: 'Application bar' }).getByRole('link', { name: 'Hubtask', exact: true }).count(),
    1,
    'the home link has no name',
  );

  // The mark in the bar carries the dot: there is something to read, without a count.
  const mark = markOf(page);
  assert.equal(await mark.locator('.dot').count(), 1, 'the bar does not say there is something to read');
  assert.equal(
    await mark.getByRole('button', { name: /This installation could not be read/ }).count(),
    1,
    'the mark does not name what is wrong',
  );

  // And it is where the retry is. `/installation` has no navigation entry; this is on every screen.
  await mark.locator('button').click();
  const surface = page.getByRole('dialog', { name: /This installation could not be read/ });
  await surface.waitFor({ timeout: 5_000 });
  assert.equal(await surface.getByText('This installation could not be read', { exact: false }).count(), 1);
  let pressed;
  pressing = new Promise((resolve) => { pressed = resolve; });
  answers = true;
  await surface.getByRole('button', { name: 'Try again' }).click();
  pressed();

  // No reload. The rows arrive because the manifest did - the set ones in Details, the empty
  // ones under the rail's folded More fields.
  await page.locator('[data-detail], button[aria-controls$="-more-fields"]').first().waitFor({ state: 'attached', timeout: 10_000 })
    .catch(() => assert.fail('asking again from the bar did not bring the entry back'));
  await page.keyboard.press('Escape');
  await unfoldFields(page);
  assert.deepEqual(
    await page.locator('[data-detail]').evaluateAll((rows) => rows.map((row) => row.getAttribute('data-detail')).sort()),
    [...ROWS].sort(),
  );
  // The pages' editor is its own chunk (E-3): it arrives a moment after the rows.
  await page.getByRole('textbox', { name: 'Notes' }).waitFor({ timeout: 10_000 }).catch(() => assert.fail('the notes did not come back'));
  assert.deepEqual(failures, []);
});

test('chromium: a manifest that declares no profile for the type says so, once', async (t) => {
  const browser = await sharedChromium(t);

  const context = await browser.newContext({ viewport: { width: 1280, height: 1000 } });
  t.after(() => context.close());
  // The read succeeds and the answer carries no profile for `TASK`. Every capability is refused,
  // one by one, for one reason — and the reason has one sentence, which is the server's own.
  await context.route('**/api/v1/**', (route) => (
    new URL(route.request().url()).pathname.endsWith('/api/v1/meta/capabilities')
      ? route.fulfill({ json: { ...MANIFEST, item_types: [] } })
      : stub(route)
  ));
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));

  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });

  await page.getByText('This workspace does not offer tasks.', { exact: false }).waitFor({ timeout: 5_000 })
    .catch(() => assert.fail('a type the installation does not declare is drawn as a type that carries nothing'));
  assert.equal(await page.locator('[data-detail]').count(), 0);
  // Said once. Fourteen refusals is fourteen sentences for one fact.
  assert.equal(await page.locator('.unread').count(), 1);
  // And this one is not the bar's business: nothing failed, so nothing is offered to ask again.
  assert.equal(await markOf(page).locator('.dot').count(), 0, 'an answered manifest put a dot in the bar');
  assert.deepEqual(failures, []);
});

test('chromium: a stale bearer signs the reader out, and the sign-in reads the manifest as that actor', async (t) => {
  const browser = await sharedChromium(t);

  const GOOD = 'e2e-fresh-bearer';
  const context = await browser.newContext({ viewport: { width: 1280, height: 1000 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const bearer = request.headers().authorization;

    if (path.endsWith('/api/v1/auth/sessions') && request.method() === 'POST') {
      return route.fulfill({ status: 201, json: { access_token: GOOD, refresh_token: 'e2e-fresh-refresh' } });
    }
    // The exchange carries no bearer of its own, and there is nothing to exchange: the refusal
    // has to reach `onUnauthorized` rather than be answered with an empty body.
    if (path.endsWith('/api/v1/auth/sessions:refresh')) return route.fulfill(problem(401, 'errors.unauthenticated'));
    // A credential that was presented is always verified, even on a public route
    // (`presentation/rest/Auth.go`) — so the manifest's own request is answered 401 too, and that
    // is the request that ends the session.
    if (bearer !== undefined && bearer !== `Bearer ${GOOD}`) return route.fulfill(problem(401, 'errors.unauthenticated'));

    if (path.endsWith('/api/v1/meta/capabilities')) {
      // Scoped by the caller, as `GetCapabilities` scopes it: the installation's answer when
      // nobody is signed in, the workspace's when somebody is. A client that kept the anonymous
      // one would draw an entry with no fields on a workspace that declares fourteen.
      return route.fulfill({ json: bearer ? MANIFEST : { ...MANIFEST, item_types: [] } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-stale-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-stale-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));

  // Opening the entry with a bearer the server has retired: the session ends on the way in.
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  const email = page.getByRole('textbox', { name: 'Email address' });
  await email.waitFor({ timeout: 15_000 }).catch(() => assert.fail('a stale bearer did not end the session'));

  await email.fill(ACCOUNT.email);
  await page.locator('input[type="password"]').fill('not-a-real-password');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();

  // Back where the reader was, and **whole**. Before this the manifest stayed `failed` for the
  // life of the page: the sign-in succeeded, every other read worked with the new token, and the
  // entry drew as though its type carried nothing until somebody reloaded.
  await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });
  assert.equal(new URL(page.url()).pathname, `/items/${ENTRY.id}`, 'the reader was not returned to the entry');
  await page.locator('[data-detail], button[aria-controls$="-more-fields"]').first().waitFor({ state: 'attached', timeout: 10_000 })
    .catch(() => assert.fail('the manifest was not read again as the signed-in actor'));
  await unfoldFields(page);
  assert.deepEqual(
    await page.locator('[data-detail]').evaluateAll((rows) => rows.map((row) => row.getAttribute('data-detail')).sort()),
    [...ROWS].sort(),
  );
  // Nothing is left over from the failure: the mark is quiet again.
  assert.equal(await markOf(page).locator('.dot').count(), 0, 'the bar still says the installation is unread');
  assert.deepEqual(failures, []);
});

// A refresh the server sheds for load (`429`, the address's sign-in budget spent - T-02) did not
// refuse the session: the token was never spent. The reader stays signed in, and the next
// exchange presents the same refresh token and goes on.
test('chromium: a refresh answered 429 keeps the session, and the next exchange goes on', async (t) => {
  const browser = await sharedChromium(t);

  const GOOD = 'e2e-fresh-bearer';
  const exchanges = [];
  const context = await browser.newContext({ viewport: { width: 1280, height: 1000 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const bearer = request.headers().authorization;
    if (path.endsWith('/api/v1/auth/sessions:refresh')) {
      exchanges.push(request.postDataJSON()?.refresh_token);
      return exchanges.length === 1
        ? route.fulfill(problem(429, 'errors.rate_limited'))
        : route.fulfill({ json: { access_token: GOOD, refresh_token: 'e2e-next-refresh' } });
    }
    if (bearer !== undefined && bearer !== `Bearer ${GOOD}`) return route.fulfill(problem(401, 'errors.unauthenticated'));
    if (path.endsWith('/api/v1/meta/capabilities')) return route.fulfill({ json: MANIFEST });
    return stub(route);
  });
  await context.addInitScript(() => {
    if (sessionStorage.getItem('hubtask.refresh') !== null) return;
    sessionStorage.setItem('hubtask.bearer', 'e2e-stale-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-stale-refresh');
  });
  const page = await context.newPage();

  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  for (let waited = 0; exchanges.length === 0 && waited < 100; waited++) await page.waitForTimeout(100);
  await page.waitForTimeout(500);
  assert.equal(await page.getByRole('textbox', { name: 'Email address' }).count(), 0, 'a shed refresh signed the reader out');

  // The next refusal, of this page or after a reload, exchanges the same unspent token.
  await page.reload();
  await page.getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });
  assert.equal(await page.getByRole('textbox', { name: 'Email address' }).count(), 0);
  assert.deepEqual(exchanges.slice(0, 2), ['e2e-stale-refresh', 'e2e-stale-refresh'], 'the second exchange presented another token');
  assert.equal(await page.evaluate(() => sessionStorage.getItem('hubtask.refresh')), 'e2e-next-refresh');
});
