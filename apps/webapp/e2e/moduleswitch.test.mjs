// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A module switched on is drawn on at once, without a reload (UC-INS-21), and a module's page
// reached while it is off says so in place of its forms - no read refused, no form that fails -
// with the switch at hand. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { productWalk } from './productwalk.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

/** The workspace's switches, kept in memory as the server keeps them. */
function modulesBackend(off) {
  const states = ['PLANNING', 'TIME', 'SERVICE_DESK'].map((module) => ({ module, on: !off.includes(module), source: 'DEFAULT' }));
  return (method, path) => {
    if (path === '/tenant/modules' && method === 'GET') return { json: { modules: states } };
    const one = /^\/tenant\/modules\/([A-Z_]+)$/.exec(path);
    if (one && method === 'PUT') {
      const state = states.find((each) => each.module === one[1]);
      Object.assign(state, { on: true, source: 'WORKSPACE' });
      return { json: { modules: states } };
    }
    return undefined;
  };
}

test('chromium: a module switched on is drawn on without a reload', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, { modules: [], answer: modulesBackend(['PLANNING', 'TIME', 'SERVICE_DESK']) });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/administration/modules`);
  for (const module of ['PLANNING', 'TIME']) {
    const row = page.locator(`[data-module="${module}"]`);
    const control = row.getByRole('switch');
    await control.waitFor({ timeout: 10_000 });
    assert.equal(await control.isChecked(), false, `${module} starts off`);
    await row.locator('label').first().click();
    await row.locator(`[data-reset="${module}"]`).waitFor({ timeout: 5_000 });
    assert.equal(await control.isChecked(), true, `${module} is drawn off after its write switched it on`);
  }
  assert.deepEqual(walk.errors(), []);
});

test('chromium: the service desk page with the module off says so, reads nothing and offers the switch', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, { modules: [], answer: modulesBackend(['SERVICE_DESK']) });
  t.after(walk.close);
  const { page } = walk;
  const asked = [];
  page.on('request', (request) => asked.push(new URL(request.url()).pathname));

  await page.goto(`${served.origin}/administration/service-desk`);
  await page.getByText('Service desk is switched off in this workspace').waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('main form').count(), 0, 'a form is offered that the server refuses');
  assert.equal(await page.getByRole('navigation', { name: 'Where you are in the administration' }).count(), 1, 'no trail back to the administration');
  assert.deepEqual(asked.filter((path) => /business-calendars|sla-policies|reply-templates|requester-organisations/.test(path)), [], 'the desk was read while off');

  await page.getByRole('button', { name: 'Switch Service desk on' }).click();
  await page.waitForTimeout(300);
  assert.ok(walk.written.some((each) => each.method === 'PUT' && each.path === '/tenant/modules/SERVICE_DESK'), 'the switch was not sent');
  assert.deepEqual(walk.errors(), []);
});
