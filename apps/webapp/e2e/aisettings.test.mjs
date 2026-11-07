// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The workspace's AI screen, walked as an administrator meets it.
//
// UC-AI-03 check 2: the consent names who receives the content and where, in words - not "the
// provider". Against a stubbed API: what is asserted is what the screen shows. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { AI_SOURCE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const ownProvider = {
  kind: 'OPENAI_COMPATIBLE', base_url: 'https://models.example.org/v1', completion_model: 'chat',
  has_api_key: true, jurisdiction: 'EEA', processing_allowed: false,
  created_at: '2026-09-01T00:00:00Z', version: 1,
};

async function open(browser, routes = {}) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const sent = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (routes[path]) return routes[path](route, request, sent);
    if (path === '/ai-provider' && request.method() === 'GET') return route.fulfill({ json: ownProvider });
    if (path === '/ai-source' && request.method() === 'GET') return route.fulfill({ json: { ...AI_SOURCE, own_configured: true } });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  return { page, sent, close: () => context.close() };
}

// UC-AI-02 check 4: the provider's declaration is shown where the workspace consents.
test('chromium: the consent names who receives the content and where', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/administration/ai`);
  const consent = page.getByRole('switch', { name: /Send this workspace/ });
  await consent.waitFor({ timeout: 15_000 });
  const label = (await consent.getAttribute('aria-label')) ?? (await page.getByText(/Send this workspace/).first().textContent());
  assert.match(label ?? '', /models\.example\.org/, `the consent does not name the receiver: ${label}`);
  assert.match(label ?? '', /in the EEA/, `the consent does not say where: ${label}`);
});

const LARGE = { id: '01a0e2e0-0000-7000-8000-0000000006a1', display_name: 'Large (EU)', processor: 'Example AI SAS', jurisdiction: 'EEA' };

/** A workspace's AI source under a rule, as `GET /ai-source` answers it. */
const sourceUnder = (sources, extra = {}) => ({
  source: 'OWN', sources, installation_sources: sources, sources_origin: 'INSTANCE', min_jurisdiction: 'ANY',
  min_jurisdiction_origin: 'DEFAULT', offered: [LARGE], search_rebuilding: false, version: 1, own_configured: true, ...extra,
});

function sourceRoutes(answer) {
  let current = answer;
  return {
    '/ai-source': (route, request, sent) => {
      if (request.method() === 'PUT') {
        const body = request.postDataJSON();
        sent.push(body);
        current = { ...current, source: body.source, offered_model_id: body.offered_model_id };
      }
      return route.fulfill({ json: current });
    },
    '/quotas': (route) => route.fulfill({ json: [
      { quota: 'ai_tokens_per_day', limit: 200000, used: 1234 },
      { quota: 'ai_own_tokens_per_day', limit: 0, used: 56 },
    ] }),
  };
}

// D1 (UC-AI-06 check 8): nothing offered, nothing decided - today's screen, and no word of an
// offered model, a lock or a minimum.
test('chromium: a private installation sees only its own model, as always', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/administration/ai`);
  await page.getByLabel(/^Provider/).waitFor({ timeout: 15_000 });
  const text = await page.locator('main').textContent();
  assert.doesNotMatch(text ?? '', /your provider|offered|This workspace may use|must process content/i);
});

// The head says what answers, and an offered model the installation withdrew is the one state
// that broke: nothing answers, said in the head and counted on the column's *AI* row
// (user-experience.md §7.10).
test('chromium: a withdrawn offered model is said in the head and counted in the column', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, sourceRoutes(sourceUnder('OFFERED', { source: 'OFFERED' })));
  t.after(close);

  await page.goto(`${served.origin}/administration/people`);
  const column = page.getByRole('navigation', { name: 'Administration' });
  await column.getByText('Nothing answers').waitFor({ timeout: 15_000 });
  assert.equal(await column.getByRole('treeitem', { name: /Nothing answers/ }).count(), 1, 'the figure is not heard with the row');

  await page.goto(`${served.origin}/administration/ai`);
  await page.locator('[data-ai-answering="withdrawn"]').getByText('the offered model was withdrawn', { exact: false }).waitFor({ timeout: 15_000 });
});

// D5 (UC-AI-05 checks 1, 2; UC-AI-02 check 5): the offered model locked - one card, one switch,
// processor and where in words, and no own form. Switching it on is the consent, one request.
test('chromium: a consumer switches on the offered model with one control', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser, sourceRoutes(sourceUnder('OFFERED', { sources_lock: 'INSTANCE' })));
  t.after(close);

  await page.goto(`${served.origin}/administration/ai`);
  const use = page.getByRole('switch', { name: 'Use Large (EU)' });
  await use.waitFor({ timeout: 15_000 });
  assert.match(await page.locator('main').textContent() ?? '', /sent to Example AI SAS in the EEA when somebody asks/);
  assert.equal(await page.getByLabel(/^Provider/).count(), 0, 'an own model is offered where only the provider’s may be used');
  assert.equal(await page.getByLabel('This workspace may use').count(), 0, 'a locked rule is offered for narrowing');

  await use.check();
  for (let i = 0; i < 50 && sent.length === 0; i++) await page.waitForTimeout(100);
  assert.deepEqual(sent[0], { source: 'OFFERED', offered_model_id: LARGE.id });
  assert.ok(await use.isChecked(), 'the switch does not show the model in use');

  // The budget is its own tab (ux-inventory §E8): only the offered model's, and no word of an own
  // one (P-05).
  await page.locator('[data-settings-sections]').getByRole('link', { name: 'Budget', exact: true }).click();
  await page.waitForFunction(() => location.search === '?tab=budget');
  await page.getByRole('heading', { name: 'The budget' }).waitFor();
  assert.match(await page.locator('main').textContent() ?? '', /The offered model: today 1,234 tokens of 200,000 tokens used/);
  assert.doesNotMatch(await page.locator('main').textContent() ?? '', /Our own model:/);
  assert.equal(await page.getByLabel(/^Daily budget of our own model/).count(), 0);
});

// UC-AI-05 check 6: where the installation offers several models, the workspace picks one by name -
// each named with its processor and where, and the choice sends that model and no other.
test('chromium: of several offered models the workspace picks one by name', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const SMALL = { id: '01a0e2e0-0000-7000-8000-0000000006a2', display_name: 'Small (DE)', processor: 'Beispiel KI GmbH', jurisdiction: 'SELF_HOSTED' };
  const { page, sent, close } = await open(browser, sourceRoutes(sourceUnder('OFFERED', { sources_lock: 'INSTANCE', offered: [LARGE, SMALL] })));
  t.after(close);

  await page.goto(`${served.origin}/administration/ai`);
  const large = page.getByRole('switch', { name: 'Use Large (EU)' });
  const small = page.getByRole('switch', { name: 'Use Small (DE)' });
  await small.waitFor({ timeout: 15_000 });
  assert.equal(await large.count(), 1, 'the first offered model is not named');
  const text = await page.locator('main').textContent() ?? '';
  assert.match(text, /Example AI SAS/);
  assert.match(text, /Beispiel KI GmbH/);

  await small.check();
  for (let i = 0; i < 50 && sent.length === 0; i++) await page.waitForTimeout(100);
  assert.deepEqual(sent[0], { source: 'OFFERED', offered_model_id: SMALL.id });
  assert.ok(await small.isChecked(), 'the chosen model is not shown in use');
  assert.equal(await large.isChecked(), false, 'the other model is shown in use too');
});

// D6 (UC-AI-05 check 7, UC-AI-06 check 4): either source on one screen, the own form kept, and the
// minimum said with who set it.
test('chromium: a company chooses between the offered model and its own', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser, sourceRoutes(sourceUnder('EITHER', {
    min_jurisdiction: 'EEA', min_jurisdiction_origin: 'INSTANCE',
  })));
  t.after(close);

  await page.goto(`${served.origin}/administration/ai`);
  await page.getByRole('switch', { name: 'Use Large (EU)' }).waitFor({ timeout: 15_000 });
  const own = page.getByRole('switch', { name: 'Use our own model' });
  assert.ok(await own.isChecked(), 'the own model in use is not shown as chosen');
  await page.getByLabel(/^Provider/).waitFor();
  assert.match(await page.locator('main').textContent() ?? '', /must process content in the EEA: set for every workspace on this installation/);
  // A declaration the minimum refuses is not offered (P-05).
  const offeredWhere = await page.getByLabel(/^Where the provider processes/).locator('option').allTextContents();
  assert.ok(!offeredWhere.some((each) => /adequacy|third country/i.test(each)), offeredWhere.join(' | '));
  await page.getByLabel('This workspace may use').waitFor();

  await page.getByRole('switch', { name: 'Use Large (EU)' }).check();
  for (let i = 0; i < 50 && sent.length === 0; i++) await page.waitForTimeout(100);
  assert.deepEqual(sent[0], { source: 'OFFERED', offered_model_id: LARGE.id });
});

// UC-AI-07 checks 2 and 4, UC-AI-06 check 6: either source allowed - each source's use and cap side
// by side, and the workspace sets its own model's budget here, an empty field being no budget.
test('chromium: each source shows its own budget, and the workspace sets its own', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const budgets = [];
  const routes = sourceRoutes(sourceUnder('EITHER'));
  // Who set each budget, as *Limits* says it: beside the budget too (ux-inventory.md §E8).
  routes['/quotas'] = (route) => route.fulfill({ json: [
    { quota: 'ai_tokens_per_day', limit: 200000, used: 1234, source: 'INSTANCE', lock: null },
    { quota: 'ai_own_tokens_per_day', limit: 0, used: 56, source: 'WORKSPACE', lock: null },
  ] });
  routes['/ai-budget'] = (route, request) => {
    budgets.push(request.postDataJSON());
    return route.fulfill({ json: [
      { quota: 'ai_tokens_per_day', limit: 200000, used: 1234 },
      { quota: 'ai_own_tokens_per_day', limit: 5000, used: 56, configured: true },
    ] });
  };
  const { page, close } = await open(browser, routes);
  t.after(close);

  await page.goto(`${served.origin}/administration/ai?tab=budget`);
  await page.getByRole('heading', { name: 'The budget' }).waitFor({ timeout: 15_000 });
  assert.equal(await page.getByRole('switch', { name: 'Use Large (EU)' }).count(), 0, 'the source is drawn on the budget tab');
  const text = await page.locator('main').textContent() ?? '';
  assert.match(text, /The offered model: today 1,234 tokens of 200,000 tokens used/);
  assert.match(text, /Our own model: today 56 tokens used, with no budget/);
  assert.match(text, /in this workspace's time zone/);
  assert.equal(await page.locator('[data-budget="offered"] [data-source]').getAttribute('data-source'), 'INSTANCE');
  assert.equal(await page.locator('[data-budget="own"] [data-source]').getAttribute('data-source'), 'WORKSPACE');

  const field = page.getByLabel(/^Daily budget of our own model/);
  await field.fill('5000');
  await page.getByRole('button', { name: 'Save the budget' }).click();
  for (let i = 0; i < 50 && budgets.length === 0; i++) await page.waitForTimeout(100);
  assert.deepEqual(budgets[0], { own_tokens_per_day: 5000 });

  await field.fill('');
  await page.getByRole('button', { name: 'Save the budget' }).click();
  for (let i = 0; i < 50 && budgets.length < 2; i++) await page.waitForTimeout(100);
  assert.deepEqual(budgets[1], { own_tokens_per_day: null });
});

// The AI screen stands on the settings shell one section at a time (user-experience.md §5.4): a
// press opens the section at its own address with the keyboard on its heading, and a consent
// flipped but not saved is held by the leave guard.
test('chromium: the AI sections open at their address and an unsaved consent is asked about before leaving', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/administration/ai`);
  const sections = page.getByRole('navigation', { name: 'Sections of AI' });
  await sections.getByRole('link', { name: 'Budget', exact: true }).click();
  await page.waitForFunction(() => location.search === '?tab=budget');
  await page.waitForFunction(() => document.activeElement?.textContent?.trim() === 'The budget', null, { timeout: 5_000 });
  assert.equal(await sections.locator('a[aria-current="page"]').textContent(), 'Budget');

  await sections.getByRole('link', { name: 'Source', exact: true }).click();
  const consent = page.getByRole('switch', { name: /Send this workspace/ });
  await consent.click();
  await page.evaluate(() => {
    const link = document.createElement('a');
    link.href = '/administration/voice';
    document.body.append(link);
    link.click();
    link.remove();
  });
  const question = page.getByRole('dialog', { name: 'Leave without saving?' });
  await question.waitFor({ timeout: 5_000 });
  await question.getByRole('button', { name: 'Stay' }).last().click();
  assert.equal(new URL(page.url()).pathname, '/administration/ai');
  await consent.click();
  await page.evaluate(() => {
    const link = document.createElement('a');
    link.href = '/administration/voice';
    document.body.append(link);
    link.click();
    link.remove();
  });
  await page.waitForURL('**/administration/voice', { timeout: 5_000 });
});

// UC-AI-02 checks 1 and 2, the screen's part: the stored provider is changed and removed here, the
// key never comes back - the field stays empty and says a key is stored - and a change that does not
// retype the key sends none, so the sealed one is kept.
test('chromium: the provider is changed and removed on the screen, and its key is never shown', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const writes = [];
  const { page, close } = await open(browser, {
    '/ai-provider': (route, request) => {
      if (request.method() === 'GET') return route.fulfill({ json: ownProvider });
      writes.push({ method: request.method(), body: request.method() === 'PUT' ? request.postDataJSON() : undefined });
      return request.method() === 'DELETE' ? route.fulfill({ status: 204 }) : route.fulfill({ json: { ...ownProvider, ...request.postDataJSON(), has_api_key: true, version: 2 } });
    },
  });
  t.after(close);

  await page.goto(`${served.origin}/administration/ai`);
  const key = page.getByLabel(/^API key/);
  await key.waitFor({ timeout: 15_000 });
  assert.equal(await key.inputValue(), '', 'the stored key came back into the field');
  assert.match(await page.locator('main').textContent() ?? '', /A key is stored\./);

  const model = page.getByLabel(/^Model for proposals/);
  await model.fill('chat-large');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  for (let i = 0; i < 50 && writes.length === 0; i++) await page.waitForTimeout(100);
  assert.equal(writes[0]?.method, 'PUT');
  assert.equal(writes[0].body.completion_model, 'chat-large');
  assert.ok(!('api_key' in writes[0].body), `a change without a new key sent one: ${JSON.stringify(writes[0].body)}`);

  await page.getByRole('button', { name: 'Remove the provider', exact: true }).click();
  await page.getByRole('button', { name: 'Remove it', exact: true }).click();
  for (let i = 0; i < 50 && writes.length < 2; i++) await page.waitForTimeout(100);
  assert.equal(writes[1]?.method, 'DELETE', 'the removal reached no DELETE');
});
