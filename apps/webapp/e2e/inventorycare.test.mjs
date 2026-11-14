// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Upkeep* of a hub's inventory as the work surface draws it (§6.10, IN-4, scene invcare,
// UC-AST-06 check 6, UC-AST-07 check 7, UC-AST-10 checks 5 and 7): (1) six figures, each opening what it
// counts; (2) *To do*, one row per problem with one act - a deviation compared beside its run and
// settled, a duplicate pair decided side by side; (3) the attestation round with *Remind*; (4) the duplicates;
// (5) the sources with their runs and *By hand*, a run's decisions behind its row, connecting a
// source and importing. The former sub-pages' addresses draw their part alone; a reader is shown
// no *Upkeep*; on a phone it reads. `SHOTS=<dir>` writes the screenshots the mockup is compared with.
//
// Chromium only: what this asserts is the rendered screen.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { HUB } from './fixture.mjs';
import { CIS, HOLD, PAIR, ROUND, bar, noSidewaysScroll, openInventory } from './inventoryseed.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());
const SHOTS = process.env.SHOTS;
const flat = (text) => text.replace(/\s+/gu, ' ').trim();

test('chromium: scene invcare - six figures, the to-dos with one act each, the round, the duplicates, the sources', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  // The day of the seed's runs: the last and the next said as the reader says them.
  const { page, context, failures, written } = await openInventory(browser, served.origin, { path: `/hubs/${HUB.id}/inventory/care`, now: '2026-11-14T12:00:00Z' });
  t.after(() => context.close());

  // (1) Six figures in their order, wanting in the warning tone, each a link to what it counts.
  const figures = page.locator('[data-care-figure]');
  await figures.first().waitFor({ timeout: 20_000 });
  assert.equal(await bar(page).getByRole('link', { name: 'Upkeep' }).getAttribute('aria-current'), 'page');
  assert.deepEqual((await figures.allInnerTexts()).map(flat), [
    'Complete 100 % all required attributes', 'Correct 92 % 1 deviation from the source', 'Stale 0 after 60 days',
    'Orphaned 1 without a relation', 'Duplicates 0 to decide', 'Without responsibility 1 person missing',
  ]);
  assert.deepEqual(await figures.evaluateAll((all) => all.filter((each) => each.hasAttribute('data-wanting')).map((each) => each.dataset.careFigure)), ['correct', 'orphans', 'without_owner']);
  assert.equal(await page.locator('[data-care-figure=without_owner]').getAttribute('href'), `/hubs/${HUB.id}/inventory?line=${encodeURIComponent('asset.owner: none')}`);
  assert.equal(await page.locator('[data-care-figure=duplicates]').getAttribute('href'), `/hubs/${HUB.id}/inventory/duplicates`);
  assert.equal(await page.locator('[data-care-figure=correct]').getAttribute('href'), `/hubs/${HUB.id}/inventory/runs`);

  // (2) One row per problem, each with its one act.
  const todos = page.locator('[data-care-todos]');
  await todos.getByText('Exchange Online Connector is related to nothing').waitFor({ timeout: 5_000 });
  await todos.getByText('app-01 and app-02.hh.intern may be the same object').waitFor({ timeout: 5_000 });
  assert.deepEqual((await todos.locator('[data-care-todo]').allInnerTexts()).map(flat), [
    'ThinkPad T14 has no responsible person Assign',
    'Exchange Online Connector is related to nothing Relate',
    'The review of “E-Mail & Kalender” is overdue Check',
    'glpi reports db-01.hh.intern differently Compare',
    'app-01 and app-02.hh.intern may be the same object Decide',
  ]);
  assert.equal(await todos.getByRole('link', { name: /^Assign:/ }).getAttribute('href'), `/cis/${CIS.nb0422.id}`);
  assert.equal(await todos.getByRole('link', { name: /^Relate:/ }).getAttribute('href'), `/cis/${CIS.exchange.id}#ci-relations`);
  assert.equal(await todos.getByRole('link', { name: /^Check:/ }).getAttribute('href'), `/cis/${CIS.mail.id}`);
  // Compare: the deviation beside its run's decision, settled in place.
  await todos.getByRole('button', { name: /^Compare:/ }).click();
  const compare = page.getByRole('dialog', { name: 'Deviation reported by glpi' });
  // A dialog holds no table (the dialog ladder): the decision stands as a list.
  assert.equal(await compare.getByRole('table').count(), 0);
  await compare.locator('[data-decision-list]').getByText('db-01').waitFor({ timeout: 5_000 });
  assert.equal(await compare.locator('[data-decision-list]').getByText('app-01').count(), 0, 'the run\'s other decisions are shown');
  await compare.getByRole('button', { name: 'Settled' }).click();
  await compare.waitFor({ state: 'detached', timeout: 5_000 });
  assert.ok(written.some((each) => each.method === 'POST' && each.at === `/ci-holds/${HOLD.id}:dismiss`), 'the deviation was not closed');
  // Decide: one page deeper, the pair side by side in today's comparison (a table, so no dialog).
  const decideLink = todos.getByRole('link', { name: /^Decide:/ });
  assert.equal(await decideLink.getAttribute('href'), `/hubs/${HUB.id}/inventory/duplicates#pair-${PAIR.id}`);
  await decideLink.click();
  const decide = page.locator(`#pair-${PAIR.id}`);
  await decide.getByRole('button', { name: 'Keep this one app-02.hh.intern' }).waitFor({ timeout: 5_000 });
  assert.equal(await decide.getByRole('button', { name: 'They are distinct' }).count(), 1);
  await page.locator('[data-care-back]').click();
  await page.locator('[data-care-part=campaigns]').waitFor({ timeout: 5_000 });

  // (3) The open round with its score, *Remind* as its quiet act while objects wait, and the earlier one.
  const round = page.locator('[data-care-part=campaigns]');
  assert.match(flat(await round.innerText()), /Bestätigungsrunde Q4 Owners confirm their objects by Nov(ember)? 30, 2026 7 of 12 confirmed · 2 replies “not right” Remind Open the round 1 earlier round/);
  // The share confirmed as the scene's bar, under the day it is due.
  assert.equal(await round.locator('progress').evaluate((bar) => Math.round(bar.value)), 58);
  assert.deepEqual(await round.getByRole('button').allInnerTexts(), ['Remind']);
  await round.getByRole('button', { name: 'Remind' }).click();
  await page.getByRole('status').filter({ hasText: '2 people reminded.' }).waitFor({ state: 'attached', timeout: 5_000 });
  assert.ok(written.some((each) => each.method === 'POST' && each.at === `/ci-campaigns/${ROUND.id}:remind`), 'the round reminded nobody');

  // (4) The duplicates.
  const pairs = page.locator('[data-care-part=duplicates]');
  assert.match(flat(await pairs.innerText()), /Duplicates app-01 · app-02\.hh\.intern Decide All duplicates/);

  // (5) The sources bringing objects with their last run and the next, then those by hand; an alert intake is no source.
  const sources = page.locator('[data-care-part=connectors]');
  const rows = sources.getByRole('row');
  await sources.locator('[data-care-source="by-hand"]').waitFor({ timeout: 5_000 });
  assert.deepEqual(await sources.locator('[data-care-source]').evaluateAll((all) => all.map((each) => each.dataset.careSource)), ['GLPI', 'Intune', 'by-hand']);
  assert.match(flat(await rows.filter({ hasText: 'GLPI' }).innerText()), /GLPI today \d{1,2}:00\s?[AP]M 12 read · 1 changed tomorrow \d{1,2}:00\s?[AP]M/u);
  assert.match(flat(await rows.filter({ hasText: 'Intune' }).innerText()), /2 read · unchanged/);
  assert.match(flat(await rows.filter({ hasText: 'By hand' }).innerText()), /By hand — 4 objects —/);
  // A run's decisions behind its row.
  await rows.filter({ hasText: 'GLPI' }).getByRole('button').first().click();
  const run = page.getByRole('dialog', { name: /^Run of glpi/ });
  await run.locator('[data-decision-list]').getByText('app-01').waitFor({ timeout: 5_000 });
  await run.getByRole('button', { name: 'Close' }).first().click();
  await run.waitFor({ state: 'detached', timeout: 5_000 });
  assert.equal(await sources.getByRole('link', { name: 'Manage sources' }).getAttribute('href'), '/assets/sources');
  assert.equal(await sources.getByRole('link', { name: 'Import' }).getAttribute('href'), `/hubs/${HUB.id}/inventory/import`);
  // *Connect a source* and *Import* are the scene's two equal quiet acts.
  const look = (locator) => locator.evaluate((each) => { const style = getComputedStyle(each); return [style.fontSize, style.fontWeight, style.color]; });
  assert.deepEqual(await look(sources.getByRole('link', { name: 'Import' })), await look(sources.getByRole('button', { name: 'Connect a source' })));
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/invcare.1280.png`, fullPage: true });
  // Connecting a source is today's connector dialog on its part.
  await sources.getByRole('button', { name: 'Connect a source' }).click();
  await page.getByRole('dialog').getByLabel('Program').waitFor({ timeout: 5_000 });
  assert.ok(page.url().endsWith('/inventory/connectors'));
  assert.deepEqual(failures, []);
});

test('chromium: the former sub-pages draw their part alone under Back to upkeep', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await openInventory(browser, served.origin, { path: `/hubs/${HUB.id}/inventory/runs` });
  t.after(() => context.close());

  for (const [part, heading] of [['runs', 'Runs'], ['duplicates', 'Duplicates'], ['connectors', 'Connectors'], ['campaigns', 'Campaigns'], ['import', 'Import']]) {
    await page.goto(`${served.origin}/hubs/${HUB.id}/inventory/${part}`);
    const alone = page.locator(`section#care-${part}[data-care-part=${part}]`);
    await alone.locator(`#care-${part}-heading`).filter({ hasText: heading }).waitFor({ timeout: 10_000 });
    assert.equal(await page.locator('[data-care-figures]').count(), 0, `/${part} draws the overview`);
    assert.equal(await bar(page).getByRole('link', { name: 'Upkeep' }).getAttribute('aria-current'), 'page');
  }
  await page.locator('[data-care-back]').click();
  await page.locator('[data-care-figures]').waitFor({ timeout: 5_000 });
  assert.ok(page.url().endsWith('/inventory/care'));
  assert.deepEqual(failures, []);
});

test('chromium: the round\'s part page reminds the owners it waits for', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures, written } = await openInventory(browser, served.origin, { path: `/hubs/${HUB.id}/inventory/campaigns` });
  t.after(() => context.close());

  const row = page.locator(`[data-campaign="${ROUND.id}"]`);
  await row.waitFor({ timeout: 20_000 });
  await row.getByRole('button').first().click();
  const detail = page.locator('[data-campaign-detail]');
  await detail.locator('[data-attestation]').first().waitFor({ timeout: 5_000 });
  const remind = detail.locator('[data-remind-round]');
  assert.equal(await remind.innerText(), 'Remind');
  await remind.click();
  await page.getByRole('status').filter({ hasText: '2 people reminded.' }).waitFor({ state: 'attached', timeout: 5_000 });
  assert.equal(written.filter((each) => each.at === `/ci-campaigns/${ROUND.id}:remind`).length, 1);
  assert.deepEqual(failures, []);
});

test('chromium: a reader is shown no Upkeep, and its address shows Stock', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await openInventory(browser, served.origin, { path: `/hubs/${HUB.id}/inventory/care`, role: 'VIEWER' });
  t.after(() => context.close());

  await bar(page).getByRole('link', { name: 'Stock' }).waitFor({ timeout: 20_000 });
  assert.equal(await bar(page).getByRole('link', { name: 'Stock' }).getAttribute('aria-current'), 'page');
  assert.equal(await bar(page).getByRole('link', { name: 'Upkeep' }).count(), 0);
  assert.equal(await page.locator('[data-care]').count(), 0);
  assert.deepEqual(failures, []);
});

test('chromium 390 px: scene invcare on a phone reads - figures, to-dos and sources without a write', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await openInventory(browser, served.origin, { width: 390, height: 844, path: `/hubs/${HUB.id}/inventory/care` });
  t.after(() => context.close());

  await page.locator('[data-care-figure]').first().waitFor({ timeout: 20_000 });
  await page.locator('[data-care-source="by-hand"]').waitFor({ timeout: 5_000 });
  await noSidewaysScroll(page, 'upkeep on a phone', assert);
  // A thumb's target: the last pair's *Decide* and *All duplicates* below it each keep their own press -
  // pressed anywhere within layout.target of its centre, each answers for itself, never for the other.
  const pairs = page.locator('#care-duplicates');
  const decide = pairs.getByRole('link', { name: /^Decide:/ }).last();
  const all = pairs.getByRole('link', { name: 'All duplicates' });
  const pressedAt = (link, dy) => link.evaluate((element, offset) => {
    const box = element.getBoundingClientRect();
    const hit = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2 + offset);
    return hit?.closest('a')?.textContent?.trim() ?? null;
  }, dy);
  await decide.scrollIntoViewIfNeeded();
  for (const dy of [-21, 0, 21]) assert.equal(await pressedAt(decide, dy), 'Decide', `Decide pressed at ${dy}`);
  for (const dy of [-21, 0, 21]) assert.equal(await pressedAt(all, dy), 'All duplicates', `All duplicates pressed at ${dy}`);
  for (const name of ['Start a round', 'Remind', 'Connect a source']) assert.equal(await page.getByRole('button', { name }).count(), 0, `${name} on a phone`);
  assert.equal(await page.locator('[data-care]').getByRole('link', { name: 'Import' }).count(), 0, 'Import on a phone');
  await page.getByRole('button', { name: /^Compare:/ }).click();
  const compare = page.getByRole('dialog', { name: 'Deviation reported by glpi' });
  await compare.waitFor({ timeout: 5_000 });
  assert.equal(await compare.getByRole('button', { name: 'Settled' }).count(), 0, 'a deviation is settled on a phone');
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/invcare.390.png`, fullPage: true });
  assert.deepEqual(failures, []);
});
