// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Stock › Devices and licences* as the work surface draws it (§6.10, scene invreg, UC-AST-12
// check 5, UC-AST-13 check 3): (1) the register as a calm table - number, device, with whom, state,
// cost, warranty in the warning tone when it ends within six months - with the value issued and the
// warranties ending beside the switch; (2) a row opens the device's life beside the table, its stock
// facts and the moves its state allows - disposing with its proof in a Dialog M -, its address
// naming it. The software as one table; a reader moves nothing; on a phone the device opens as a
// sheet that reads. `SHOTS=<dir>` writes the screenshots the mockup is compared with.
//
// Chromium only: what this asserts is the rendered screen.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { serve } from './serve.mjs';
import { HUB } from './fixture.mjs';
import { CIS, bar, noSidewaysScroll, openInventory } from './inventoryseed.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());
const SHOTS = process.env.SHOTS;
const NOW = '2026-11-14T10:00:00Z';
const flat = (text) => text.replace(/\s+/gu, ' ').trim();
const REGISTER = `/hubs/${HUB.id}/inventory/register`;

test('chromium: scene invreg - the register table, a device\'s life beside it and the moves its state allows', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures, written } = await openInventory(browser, served.origin, { path: REGISTER, now: NOW });
  t.after(() => context.close());

  // (1) The table under Stock's switch, the summary beside it.
  const table = page.locator('[data-register]');
  await table.getByRole('row').nth(5).waitFor({ timeout: 20_000 });
  assert.equal(await bar(page).getByRole('link', { name: 'Stock' }).getAttribute('aria-current'), 'page');
  assert.equal(await page.getByRole('radio', { name: 'Devices and licences · 5' }).getAttribute('aria-checked'), 'true');
  assert.equal(flat(await page.locator('[data-register-summary]').innerText()), 'Value in use €2,960.00 · 1 warranty ends in 4 months');
  assert.deepEqual((await table.getByRole('columnheader').allInnerTexts()).map(flat).filter(Boolean).map((each) => each.toLowerCase()), ['no.', 'device', 'with', 'state', 'cost', 'warranty']);
  const row = (tag) => table.getByRole('row').filter({ hasText: tag });
  assert.match(flat(await row('NB-0422').innerText()), /NB-0422 Laptop · ThinkPad T14 .*Issued €1,480\.00 3\/2027/);
  assert.match(flat(await row('TL-0102').innerText()), /TL-0102 Phone · Pixel 9 — Ordered €799\.00 —/);
  // The warranty ending within six months is drawn as ending; the far one and the retired device's are not.
  assert.equal(await row('NB-0422').locator('[data-warranty]').getAttribute('data-warranty'), 'warning');
  assert.equal(await row('NB-0417').locator('[data-warranty]').getAttribute('data-warranty'), 'neutral');
  assert.equal(await row('NB-0233').locator('[data-warranty]').getAttribute('data-warranty'), 'neutral');
  assert.equal(await row('NB-0422').getByRole('cell').nth(4).evaluate((cell) => getComputedStyle(cell).textAlign), 'end');
  assert.equal(await row('NB-0422').locator('.tag').evaluate((each) => getComputedStyle(each).fontFamily.toLowerCase().includes('mono')), true);

  // (2) A row opens the device beside the table: its number, name and holder, its life, its stock facts.
  await row('NB-0422').getByRole('link').first().click();
  await page.waitForFunction((at) => location.search === `?asset=${at}`, CIS.nb0422.id, { timeout: 5_000 });
  const panel = page.locator(`[data-register-panel="${CIS.nb0422.id}"]`);
  await panel.locator('[data-asset-life]').waitFor({ timeout: 5_000 });
  assert.match(flat(await panel.getByRole('heading', { level: 2 }).innerText()), /^NB-0422 ThinkPad T14 · /);
  assert.deepEqual(await panel.locator('[data-life-step][data-reached]').evaluateAll((all) => all.map((each) => each.dataset.lifeStep)), ['REQUESTED', 'ORDERED', 'IN_STOCK', 'DEPLOYED']);
  assert.equal(await panel.locator('[data-life-step][data-current]').getAttribute('data-life-step'), 'DEPLOYED');
  assert.match(flat(await panel.locator('[data-life-step=DISPOSED]').innerText()), /Disposed of .*Proof needed/);
  await panel.getByText('Rahmenvertrag Hardware 2025').waitFor({ timeout: 5_000 });
  assert.match(flat(await panel.locator('dl').innerText()), /Cost €1,480\.00 Purchased on Mar 14, 2025 Warranty until March 2027 Contract Rahmenvertrag Hardware 2025 Stockroom Lager HH/);
  const boxes = await Promise.all([table.boundingBox(), panel.boundingBox()]);
  assert.ok(boxes[1].x >= boxes[0].x + boxes[0].width, 'the device does not stand beside the table');
  // The moves an issued device allows; retiring is sent.
  assert.deepEqual((await panel.locator('[data-register-acts]').getByRole('button').allInnerTexts()).map(flat), ['Back to stock', 'Retire']);
  await panel.getByRole('button', { name: 'Retire' }).click();
  await page.getByText('The register record is now Retired.').first().waitFor({ timeout: 5_000 });
  assert.ok(written.some((each) => each.at === `/cis/${CIS.nb0422.id}/asset:move` && each.body?.state === 'RETIRED'), 'retiring was not sent');
  // The device stays open, now retired, its moves those a retired one allows.
  await panel.getByRole('button', { name: 'Dispose of' }).waitFor({ timeout: 5_000 });
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/invreg.1280.png`, fullPage: true });

  // A retired device goes back or is disposed of - with its proof, in a Dialog M.
  await row('NB-0233').getByRole('link').first().click();
  const retired = page.locator(`[data-register-panel="${CIS.nb0233.id}"]`);
  await retired.locator('[data-register-acts]').waitFor({ timeout: 5_000 });
  assert.deepEqual((await retired.locator('[data-register-acts]').getByRole('button').allInnerTexts()).map(flat), ['Back to stock', 'Dispose of']);
  await retired.getByRole('button', { name: 'Dispose of' }).click();
  const dispose = page.getByRole('dialog', { name: 'Dispose of it' });
  await dispose.getByLabel('Certificate').fill('WEEE-2026-118');
  await dispose.getByLabel('Method').selectOption({ label: 'Destroyed' });
  await dispose.getByRole('button', { name: 'Dispose of it' }).click();
  await dispose.waitFor({ state: 'detached', timeout: 5_000 });
  const disposal = written.find((each) => each.at === `/cis/${CIS.nb0233.id}/asset:move`);
  assert.deepEqual(disposal?.body, { state: 'DISPOSED', disposal: { method: 'DESTROYED', certificate: 'WEEE-2026-118' } });

  // Closing the panel leaves the table alone, its address without the device.
  await retired.getByRole('button', { name: 'Close' }).click();
  await page.waitForFunction(() => location.search === '', null, { timeout: 5_000 });
  assert.equal(await page.locator('[data-register-panel]').count(), 0);

  // The software, one table of versions under each product.
  await page.getByRole('radio', { name: /Software/ }).click();
  const software = page.locator('[data-software-table]');
  await software.locator('[data-software=Office]').waitFor({ timeout: 5_000 });
  assert.match(flat(await software.innerText()), /Office Microsoft By an alias installed on 3 hosts[\s\S]*2024 2 2[\s\S]*2021 1 1/);
  assert.deepEqual(failures, []);
});

test('chromium: a reader opens a device and its life, and is offered no move', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures } = await openInventory(browser, served.origin, { path: `${REGISTER}?asset=${CIS.nb0422.id}`, role: 'VIEWER', now: NOW });
  t.after(() => context.close());

  const panel = page.locator(`[data-register-panel="${CIS.nb0422.id}"]`);
  await panel.locator('[data-asset-life]').waitFor({ timeout: 20_000 });
  await panel.locator('dl').waitFor({ timeout: 5_000 });
  assert.equal(await panel.locator('[data-register-acts]').count(), 0);
  assert.equal(await page.locator('[data-register] button').count(), 0);
  assert.deepEqual(failures, []);
});

test('chromium 390 px: scene invreg on a phone - the device opens as a sheet that reads', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures } = await openInventory(browser, served.origin, { width: 390, height: 844, path: REGISTER, now: NOW });
  t.after(() => context.close());

  await page.locator('[data-register]').getByRole('row').nth(3).waitFor({ timeout: 20_000 });
  await noSidewaysScroll(page, 'the register on a phone', assert);
  await page.locator('[data-register]').getByRole('row').filter({ hasText: 'NB-0422' }).getByRole('link').first().click();
  const sheet = page.getByRole('dialog');
  await sheet.locator('[data-asset-life]').waitFor({ timeout: 5_000 });
  await sheet.locator('dl').waitFor({ timeout: 5_000 });
  assert.equal(await sheet.locator('[data-register-acts]').count(), 0, 'a phone moves a device');
  await noSidewaysScroll(page, 'the device on a phone', assert);
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/invreg.390.png` });
  assert.deepEqual(failures, []);
});
