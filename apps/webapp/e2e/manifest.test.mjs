// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The entry screen when `/meta/capabilities` has not been read (issue 1020). Everything an entry
// shows below its title is the manifest's answer, and the screen used to render "not read yet"
// exactly like "this type refuses it": no rows, no notes, no subtree, and not a word about why.
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

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, ITEMS, MANIFEST, stub } from './fixture.mjs';

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
  const browser = await chromium.launch();
  t.after(() => browser.close());

  // The read fails until the walk says otherwise, which is what the retry is measured against.
  let answers = false;
  const context = await browser.newContext({ viewport: { width: 1280, height: 1000 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', (route) => {
    if (new URL(route.request().url()).pathname.endsWith('/api/v1/meta/capabilities')) {
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
  await page.getByRole('textbox', { name: 'Title' }).first().waitFor({ timeout: 15_000 });

  // The entry's own values are there — they are the entry's, not the type's.
  assert.equal(await page.getByRole('textbox', { name: 'Title' }).first().inputValue(), ENTRY.title);

  // **Nothing is claimed about the type.** Not one row, and not the language row either: the
  // picker's own list of languages comes out of the same manifest, so a row offered here would be
  // a row with nothing in it.
  const drawn = await page.locator('[data-detail]').evaluateAll((rows) => rows.map((row) => row.getAttribute('data-detail')));
  assert.deepEqual(drawn, [], `the column drew ${JSON.stringify(drawn)} from a manifest nobody has`);

  // And it is said. Before this the column was simply empty, which reads exactly like a type that
  // carries nothing (voice-and-tone.md §4.4).
  const said = page.getByText('What this kind of entry can hold could not be read', { exact: false });
  await said.waitFor({ timeout: 5_000 }).catch(() => assert.fail('the column says nothing about why it is empty'));
  // The server's own sentence beside it, rather than this screen's paraphrase of a status code.
  assert.equal(await page.locator('.unread-detail').count() >= 1, true, 'the reason the server gave is not shown');

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
  answers = true;
  await surface.getByRole('button', { name: 'Try again' }).click();

  // No reload. The rows arrive because the manifest did.
  await page.locator('[data-detail="due"]').waitFor({ timeout: 10_000 })
    .catch(() => assert.fail('asking again from the bar did not bring the entry back'));
  await page.keyboard.press('Escape');
  assert.deepEqual(
    await page.locator('[data-detail]').evaluateAll((rows) => rows.map((row) => row.getAttribute('data-detail'))),
    ROWS,
  );
  assert.equal(await page.getByRole('textbox', { name: 'Notes' }).count(), 1, 'the notes did not come back');
  assert.deepEqual(failures, []);
});

test('chromium: a manifest that declares no profile for the type says so, once', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

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
  await page.getByRole('textbox', { name: 'Title' }).first().waitFor({ timeout: 15_000 });

  await page.getByText('This workspace does not offer Task entries', { exact: false }).waitFor({ timeout: 5_000 })
    .catch(() => assert.fail('a type the installation does not declare is drawn as a type that carries nothing'));
  assert.equal(await page.locator('[data-detail]').count(), 0);
  // Said once. Fourteen refusals is fourteen sentences for one fact.
  assert.equal(await page.locator('.unread').count(), 1);
  // And this one is not the bar's business: nothing failed, so nothing is offered to ask again.
  assert.equal(await markOf(page).locator('.dot').count(), 0, 'an answered manifest put a dot in the bar');
  assert.deepEqual(failures, []);
});

test('chromium: a stale bearer signs the reader out, and the sign-in reads the manifest as that actor', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

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
  await page.getByRole('textbox', { name: 'Title' }).first().waitFor({ timeout: 15_000 });
  assert.equal(new URL(page.url()).pathname, `/items/${ENTRY.id}`, 'the reader was not returned to the entry');
  await page.locator('[data-detail="due"]').waitFor({ timeout: 10_000 })
    .catch(() => assert.fail('the manifest was not read again as the signed-in actor'));
  assert.deepEqual(
    await page.locator('[data-detail]').evaluateAll((rows) => rows.map((row) => row.getAttribute('data-detail'))),
    ROWS,
  );
  // Nothing is left over from the failure: the mark is quiet again.
  assert.equal(await markOf(page).locator('.dot').count(), 0, 'the bar still says the installation is unread');
  assert.deepEqual(failures, []);
});
