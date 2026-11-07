// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A type's form, its conditions folded (user-experience.md UX-4): each field's conditions sit
// under *Conditions*, picked as field, comparison and value; the expression is behind *Advanced*,
// open only for one no pick wrote.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { PAGE } from './fixture.mjs';
import { productWalk } from './productwalk.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const incident = {
  key: 'incident', level: 'TASK', system: false, position: 1, archived: false, name: 'Incident',
  form: {
    fields: [
      { key: 'title', required: true },
      { key: 'custom_fields.severity' },
      { key: 'custom_fields.impact', required_when: 'actor.role == "AGENT"' },
    ],
    channels: [{ channel: 'PORTAL' }],
  },
};
const fields = [
  { id: 'cf-1', collection_id: null, hub_id: null, key: 'severity', name: 'Severity', kind: 'SELECT', options: ['low', 'high'], is_required: false, restricted: false, read_restricted: false, applies_to: ['TASK'] },
  { id: 'cf-2', collection_id: null, hub_id: null, key: 'impact', name: 'Impact', kind: 'TEXT', options: [], is_required: false, restricted: false, read_restricted: false, applies_to: ['TASK'] },
];

test('chromium: a form field\'s conditions are folded, picked in words, the expression behind Advanced', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, {
    modules: ['WORKFLOWS'],
    answer: (method, path) => {
      if (path === '/entry-types' && method === 'GET') return { json: { ...PAGE, data: [incident] } };
      if (path.startsWith('/custom-fields') && method === 'GET') return { json: fields };
      return undefined;
    },
  });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/administration/types`);
  await page.locator('[data-edit-form="incident"]').click();
  const dialog = page.getByRole('dialog', { name: 'Form of Incident' });
  await dialog.waitFor({ timeout: 10_000 });

  const severity = dialog.locator('[data-form-field="custom_fields.severity"]');
  await severity.waitFor({ timeout: 5_000 });
  // Folded: no condition and no expression syntax is in sight until *Conditions* is opened.
  assert.equal(await dialog.getByText('A condition over entry', { exact: false }).count(), 0, 'the expression hint is open');
  assert.equal(await severity.getByRole('group', { name: 'Required when' }).count(), 0, 'the conditions are not folded');

  await severity.getByRole('button', { name: /^Conditions/ }).click();
  const required = severity.getByRole('group', { name: 'Required when' });
  await required.getByLabel('When the field').selectOption({ label: 'Severity' });
  await required.getByLabel('Compares').selectOption({ label: 'is' });
  await required.getByLabel('Value').selectOption('high');
  assert.equal(await required.getByLabel('Required when (expression)').count(), 0, 'Advanced opened by itself for a pick');
  await required.getByRole('button', { name: /^Advanced/ }).click();
  assert.equal(await required.getByLabel('Required when (expression)').inputValue(), 'entry.custom_fields.severity == "high"');

  // An expression no pick wrote opens under Advanced and is not overwritten by the picks.
  const impact = dialog.locator('[data-form-field="custom_fields.impact"]');
  await impact.getByRole('button', { name: /^Conditions/ }).click();
  const impactRequired = impact.getByRole('group', { name: 'Required when' });
  await impactRequired.getByText('Written as an expression under Advanced.').waitFor({ timeout: 5_000 });
  assert.equal(await impactRequired.getByLabel('Required when (expression)').inputValue(), 'actor.role == "AGENT"');
});
