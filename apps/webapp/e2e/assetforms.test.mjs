// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The Assets screen's forms, each opened on a value it does not hold yet (UC-AST-04 checks 2 and 6):
// a CSV table's and an Excel workbook's columns offered for the mapping, a new configuration item's
// and a refined one's attributes, a new connector's settings. A field bound to a value nobody
// has given yet starts empty: Svelte refuses to bind `undefined` to a value with a fallback, and the
// page stops drawing.
//
// Chromium only: what this asserts is the rendered forms.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { HUB, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

/** A stored (uncompressed) ZIP of the parts - all a browser's reader of a workbook needs. */
function zip(parts) {
  const locals = [];
  const centrals = [];
  let offset = 0;
  for (const [name, text] of Object.entries(parts)) {
    const nameBytes = Buffer.from(name);
    const body = Buffer.from(text);
    const local = Buffer.alloc(30 + nameBytes.length);
    local.writeUInt32LE(0x04034b50, 0);
    local.writeUInt32LE(body.length, 18);
    local.writeUInt32LE(body.length, 22);
    local.writeUInt16LE(nameBytes.length, 26);
    nameBytes.copy(local, 30);
    const central = Buffer.alloc(46 + nameBytes.length);
    central.writeUInt32LE(0x02014b50, 0);
    central.writeUInt32LE(body.length, 20);
    central.writeUInt32LE(body.length, 24);
    central.writeUInt16LE(nameBytes.length, 28);
    central.writeUInt32LE(offset, 42);
    nameBytes.copy(central, 46);
    locals.push(local, body);
    centrals.push(central);
    offset += local.length + body.length;
  }
  const size = centrals.reduce((sum, each) => sum + each.length, 0);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(centrals.length, 8);
  end.writeUInt16LE(centrals.length, 10);
  end.writeUInt32LE(size, 12);
  end.writeUInt32LE(offset, 16);
  return Buffer.concat([...locals, ...centrals, end]);
}

const WORKBOOK = zip({
  'xl/workbook.xml': '<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Assets" sheetId="1" r:id="rId1"/></sheets></workbook>',
  'xl/_rels/workbook.xml.rels': '<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>',
  'xl/worksheets/sheet1.xml': '<worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Hostname</t></is></c><c r="B1" t="inlineStr"><is><t>Serial</t></is></c></row><row r="2"><c r="A2" t="inlineStr"><is><t>web-01</t></is></c><c r="B2" t="inlineStr"><is><t>SN1</t></is></c></row></sheetData></worksheet>',
});

const FILES = [
  { name: 'assets.csv', mimeType: 'text/csv', buffer: Buffer.from('Hostname,Serial\nweb-01,SN1\n') },
  { name: 'assets.xlsx', mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', buffer: WORKBOOK },
];

for (const file of FILES) {
  test(`chromium: ${file.name}'s columns are offered for the mapping`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    t.after(() => context.close());
    // One file source an administrator defined, without a saved mapping: the import starts there.
    await context.route('**/api/v1/**', (route) => (new URL(route.request().url()).pathname.endsWith('/cmdb/sources')
      ? route.fulfill({ json: { data: [{ key: 'inventory-sheet', kind: 'FILE', rank: 1, classes: [], may_create: true, mapping: null, system: false, last_run_at: null, version: 1 }] } })
      : stub(route)));
    await context.addInitScript(() => { sessionStorage.setItem('hubtask.bearer', 'e2e-bearer'); sessionStorage.setItem('hubtask.refresh', 'e2e-refresh'); });
    const page = await context.newPage();
    const failures = [];
    page.on('pageerror', (error) => failures.push(String(error)));

    await page.goto(`${served.origin}/hubs/${HUB.id}/assets/import`);
    // The import's four steps: the source as a card, the file, then the check with the mapping.
    await page.getByRole('button', { name: 'Choose inventory-sheet' }).click({ timeout: 15_000 });
    const input = page.locator('input[type=file]');
    await input.waitFor({ state: 'attached', timeout: 5_000 });
    await input.setInputFiles(file);
    await page.getByRole('button', { name: 'Next', exact: true }).click();

    const nameColumn = page.getByLabel('Name column', { exact: false });
    await nameColumn.waitFor({ timeout: 5_000 });
    const offered = await nameColumn.locator('option').allInnerTexts();
    assert.ok(offered.includes('Hostname') && offered.includes('Serial'), `the columns are not offered: ${offered}`);
    assert.deepEqual(failures, []);
  });
}

const SERVER_CLASS = {
  key: 'server', name: 'Server', parent: 'ci', system: false, lineage: ['ci', 'server'], version: 1,
  attributes: [], identifier_rules: [], required_relations: [], staleness_days: 0,
  all_attributes: [{ key: 'serial', kind: 'TEXT', need: 'OPTIONAL' }, { key: 'rack', kind: 'TEXT', need: 'OPTIONAL' }],
};

// The same holds for a new configuration item: its class's attributes start empty, and the dialog
// asks for each of them rather than falling over.
test('chromium: a new configuration item asks for its class attributes', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', (route) => (new URL(route.request().url()).pathname.endsWith('/cmdb/classes')
    ? route.fulfill({ json: { data: [SERVER_CLASS] } })
    : stub(route)));
  await context.addInitScript(() => { sessionStorage.setItem('hubtask.bearer', 'e2e-bearer'); sessionStorage.setItem('hubtask.refresh', 'e2e-refresh'); });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));

  await page.goto(`${served.origin}/hubs/${HUB.id}/assets`);
  await page.getByRole('button', { name: 'New configuration item' }).first().click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Class', { exact: false }).first().selectOption('server');
  await dialog.getByLabel('serial', { exact: false }).waitFor({ timeout: 5_000 });
  assert.equal(await dialog.getByLabel('rack', { exact: false }).inputValue(), '');
  assert.deepEqual(failures, []);
});

const BLADE_CLASS = {
  ...SERVER_CLASS, key: 'blade', name: 'Blade', parent: 'server', lineage: ['ci', 'server', 'blade'],
  all_attributes: [...SERVER_CLASS.all_attributes, { key: 'slot', kind: 'TEXT', need: 'OPTIONAL' }],
};
const SERVER = {
  id: '01a0e2e0-0000-7000-8000-0000000c1001', hub_id: HUB.id, class: 'server', name: 'web-01', stage: 'IN_USE', status: 'OK',
  attributes: { serial: 'SN1' }, provenance: {}, protection: {}, quantity: 1,
  created_at: '2026-10-01T08:00:00Z', updated_at: '2026-10-01T08:00:00Z', version: 1,
};

// Refined to a more specific class, a configuration item is asked for the attributes it gains.
test('chromium: a configuration item refined to a narrower class asks for the attributes it gains', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith('/cmdb/classes')) return route.fulfill({ json: { data: [SERVER_CLASS, BLADE_CLASS] } });
    if (path.endsWith(`/cis/${SERVER.id}`) && route.request().method() === 'GET') return route.fulfill({ json: SERVER });
    if (path.endsWith(`/cis/${SERVER.id}/impact`)) return route.fulfill({ json: { ci_id: SERVER.id, direction: 'DOWNSTREAM', reached: [], services: [] } });
    return stub(route);
  });
  await context.addInitScript(() => { sessionStorage.setItem('hubtask.bearer', 'e2e-bearer'); sessionStorage.setItem('hubtask.refresh', 'e2e-refresh'); });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));

  await page.goto(`${served.origin}/cis/${SERVER.id}`);
  await page.getByRole('button', { name: 'Change' }).first().click();
  await page.getByLabel('More specific class').selectOption('blade');
  await page.getByLabel('slot', { exact: false }).waitFor({ timeout: 5_000 });
  assert.equal(await page.getByLabel('serial', { exact: false }).inputValue(), 'SN1');
  assert.deepEqual(failures, []);
});

// A new connector asks for its program's settings, none of which it holds yet.
test('chromium: a new connector asks for its program settings', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', stub);
  await context.addInitScript(() => { sessionStorage.setItem('hubtask.bearer', 'e2e-bearer'); sessionStorage.setItem('hubtask.refresh', 'e2e-refresh'); });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));

  await page.goto(`${served.origin}/hubs/${HUB.id}/assets/connectors`);
  await page.getByRole('button', { name: 'New connector' }).first().click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Program').selectOption('ENTRA');
  await dialog.getByLabel('Client ID').waitFor({ timeout: 5_000 });
  assert.equal(await dialog.getByLabel('Client ID').inputValue(), '');
  assert.deepEqual(failures, []);
});
