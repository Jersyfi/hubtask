// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A workspace's sign-in provider, configured as an administrator meets it (UC-ID-11 checks 1, 2,
// 4, 5, 6): the three admission answers in plain words - in English and in German - and never the
// contract's values; the list the server reads and nothing else; *New people get* only where
// somebody new arrives, defaulting to nothing; the redirect address in full, copied whole.
//
// Against a stubbed API: what is asserted is what the screen shows and what it sends. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, REDIRECT_URI as REDIRECT, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const STAFF = '01a0e2e0-0000-7000-8000-0000000007a1';

const WORDS = {
  en: {
    add: 'Add a provider', kind: 'Which provider', save: 'Save the provider',
    modes: ['Only people invited here', 'Anyone from these organisations', 'Anyone this provider knows'],
    newcomers: 'New people get', nothing: 'No access until an administrator gives it',
    group: 'Membership of the group Staff', redirect: 'Redirect address', copy: 'Copy the address',
    issuer: 'Issuer', client: 'Client id', secret: 'Client secret',
    workspaceDomains: 'Google Workspace domains that may sign in',
    entraDirectories: 'Microsoft directories that may sign in',
    mailDomains: 'Email domains this provider admits',
    emptyAdmitsNobody: /An empty list (admits|lets) nobody/,
  },
  de: {
    add: 'Anbieter hinzufügen', kind: 'Welcher Anbieter', save: 'Anbieter speichern',
    modes: ['Nur hier eingeladene Personen', 'Alle aus diesen Organisationen', 'Alle, die dieser Anbieter kennt'],
    newcomers: 'Neue Personen bekommen',
    nothing: 'Keinen Zugang, bis eine Administratorin oder ein Administrator ihn gibt',
    group: 'Mitgliedschaft in der Gruppe Staff', redirect: 'Weiterleitungsadresse', copy: 'Adresse kopieren',
    issuer: 'Aussteller', client: 'Client-ID', secret: 'Client-Geheimnis',
    workspaceDomains: 'Google-Workspace-Domänen, die sich anmelden dürfen',
    entraDirectories: 'Microsoft-Verzeichnisse, die sich anmelden dürfen',
    mailDomains: 'E-Mail-Domänen, die dieser Anbieter hereinlässt',
    emptyAdmitsNobody: /Eine leere Liste lässt niemanden herein/,
  },
};

async function open(browser, locale) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, locale });
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: served.origin });
  const sent = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/accounts/me') return route.fulfill({ json: { ...ACCOUNT, locale } });
    if (path === '/meta/capabilities') {
      return route.fulfill({
        json: {
          ...MANIFEST,
          roles: ['OWNER', 'ADMIN', 'MEMBER', 'VIEWER'].map((role) => ({ role, permissions: ['READ'] })),
        },
      });
    }
    if (path === '/identity-providers' && request.method() === 'GET') return route.fulfill({ json: [] });
    if (path === '/groups') return route.fulfill({ json: { data: [{ id: STAFF, name: 'Staff', version: 1 }] } });
    if (path === '/identity-providers' && request.method() === 'POST') {
      const body = request.postDataJSON();
      sent.push(body);
      return route.fulfill({
        status: 201,
        json: {
          id: '01a0e2e0-0000-7000-8000-0000000007b1', scope: 'workspace', issuer: body.issuer,
          client_id: body.client_id, display_name: body.issuer, kind: body.kind,
          provisioning: body.provisioning, position: 0, enabled: false, offered_here: false,
          allowed_email_domains: body.allowed_email_domains ?? [], allowed_directories: [],
          created_at: '2026-10-12T09:00:00Z', version: 1, ...(body.newcomer_group_id ? { newcomer_group_id: body.newcomer_group_id } : {}),
        },
      });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  return { page, sent, close: () => context.close() };
}

/** The labels of a select's options, in order. */
async function optionsOf(select) {
  return select.locator('option').allTextContents().then((all) => all.map((one) => one.trim()));
}

for (const locale of ['en', 'de']) {
  const words = WORDS[locale];

  test(`chromium, ${locale}: the three answers in plain words, newcomers where they arrive, the address copied whole`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, sent, close } = await open(browser, locale);
    t.after(close);

    await page.goto(`${served.origin}/administration/identity-provider`);
    await page.getByRole('button', { name: words.add }).click();
    const form = page.locator('form.panel');
    await form.waitFor();
    const kind = form.getByLabel(words.kind);
    const admission = form.getByRole('combobox').nth(1);

    // Another provider: all three answers, in the use case's words (check 1, check 3).
    await kind.selectOption('GENERIC');
    assert.deepEqual(await optionsOf(admission), words.modes);

    // Google: never *Anyone this provider knows*; by organisation it asks for Workspace domains,
    // which a personal account belongs to none of (checks 2 and 4).
    await kind.selectOption('GOOGLE');
    assert.deepEqual(await optionsOf(admission), [words.modes[0], words.modes[1]]);
    await admission.selectOption('DOMAINS');
    assert.equal(await form.getByLabel(words.workspaceDomains).count(), 1);
    assert.match(await form.innerText(), words.emptyAdmitsNobody);
    // Microsoft by organisation asks for Entra directories; any other provider for mail domains -
    // and each says that an empty list admits nobody (check 4).
    await kind.selectOption('MICROSOFT');
    await admission.selectOption('DOMAINS');
    assert.equal(await form.getByLabel(words.entraDirectories).count(), 1);
    assert.equal(await form.getByLabel(words.mailDomains).count(), 0);
    assert.match(await form.innerText(), words.emptyAdmitsNobody);
    await kind.selectOption('GENERIC');
    await admission.selectOption('DOMAINS');
    assert.equal(await form.getByLabel(words.mailDomains).count(), 1);
    assert.equal(await form.getByLabel(words.entraDirectories).count(), 0);
    assert.equal(await form.getByLabel(words.workspaceDomains).count(), 0);
    assert.match(await form.innerText(), words.emptyAdmitsNobody);

    // The contract's values are never on screen, in any preset's form.
    for (const each of ['GENERIC', 'MICROSOFT', 'GOOGLE']) {
      await kind.selectOption(each);
      const text = await form.innerText();
      assert.doesNotMatch(text, /\bINVITED_ONLY\b|\bDOMAINS\b|\bANY\b/, `${each}: ${text}`);
    }

    // The redirect address in full, and the copy control hands over exactly it (check 6).
    const redirect = form.getByRole('group', { name: words.redirect });
    assert.equal((await redirect.locator('code').textContent())?.trim(), REDIRECT);
    // In full at a phone's width too: the address wraps rather than being cut off by a field.
    await page.setViewportSize({ width: 375, height: 900 });
    const box = await redirect.locator('code').evaluate((el) => ({ scroll: el.scrollWidth, client: el.clientWidth }));
    assert.ok(box.scroll <= box.client, `the address overflows at 375 px: ${JSON.stringify(box)}`);
    await page.setViewportSize({ width: 1280, height: 900 });
    await form.getByRole('button', { name: words.copy }).click();
    assert.equal(await page.evaluate(() => navigator.clipboard.readText()), REDIRECT);

    // New people get: absent where nobody new arrives, nothing by default where they do (check 5).
    await kind.selectOption('GENERIC');
    await admission.selectOption('INVITED_ONLY');
    assert.equal(await form.getByLabel(words.newcomers).count(), 0);
    await admission.selectOption('ANY');
    const newcomers = form.getByLabel(words.newcomers);
    const offered = await optionsOf(newcomers);
    assert.equal(offered[0], words.nothing);
    assert.equal(await newcomers.inputValue(), '');
    assert.ok(offered.includes(words.group), offered.join(' | '));
    assert.ok(!offered.some((one) => /OWNER|Owner|Eigentümer/.test(one)), `OWNER offered: ${offered.join(' | ')}`);

    await newcomers.selectOption({ label: words.group });
    await form.getByLabel(new RegExp(`^${words.issuer}`)).fill('https://login.example.org');
    await form.getByLabel(new RegExp(`^${words.client}`)).fill('hubtask');
    await form.getByLabel(new RegExp(`^${words.secret}`)).fill('s3cr3t');
    await form.getByRole('button', { name: words.save }).click();
    await form.waitFor({ state: 'detached' });
    assert.equal(sent.length, 1);
    assert.equal(sent[0].provisioning, 'ANY');
    assert.equal(sent[0].newcomer_group_id, STAFF);
    assert.equal(sent[0].newcomer_role, undefined);
  });
}

// UC-ID-11: a workspace choosing Google has its one issuer filled in; another provider takes
// it out again.
test('chromium: choosing Google fills its issuer in a workspace too', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, 'en');
  t.after(close);

  await page.goto(`${served.origin}/administration/identity-provider`);
  await page.getByRole('button', { name: 'Add a provider' }).click();
  const form = page.locator('form.panel');
  const kind = form.getByLabel('Which provider');
  const issuer = form.getByLabel(/^Issuer/);
  await kind.selectOption('GOOGLE');
  assert.equal(await issuer.inputValue(), 'https://accounts.google.com');
  await kind.selectOption('GENERIC');
  assert.equal(await issuer.inputValue(), '');
});

// #1432: a shared Microsoft endpoint is saved only with its directories (identity.md §10.3), so
// under *Anyone this provider knows* the form asks for them there and sends them; a directory's own
// issuer under the same mode asks for none, and a list typed under another mode is not sent unseen.
test('chromium: a shared endpoint under Anyone asks for the directories and sends them', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser, 'en');
  t.after(close);

  await page.goto(`${served.origin}/administration/identity-provider`);
  await page.getByRole('button', { name: 'Add a provider' }).click();
  const form = page.locator('form.panel');
  await form.getByLabel('Which provider').selectOption('MICROSOFT');
  const admission = form.getByRole('combobox').nth(1);
  const directories = form.getByLabel(/^Microsoft directories that may sign in/);
  const issuer = form.getByLabel(/^Issuer/);

  // A list typed by organisation, then a directory's own issuer under Anyone: no field, nothing sent.
  await admission.selectOption('DOMAINS');
  await directories.fill('0f1e2d3c-4b5a-4968-8776-655443322110');
  await issuer.fill('https://login.microsoftonline.com/72f988bf-86f1-41af-91ab-2d7cd011db47/v2.0');
  await admission.selectOption('ANY');
  assert.equal(await directories.count(), 0, 'a directory list is drawn where the server does not read it');

  // The shared endpoint: the field is back, required, and its list is sent.
  await issuer.fill('https://login.microsoftonline.com/organizations/v2.0');
  await directories.waitFor();
  assert.match(await form.innerText(), /stands for every organisation at the provider/);
  await directories.fill('72f988bf-86f1-41af-91ab-2d7cd011db47');
  await form.getByLabel(/^Client id/).fill('hubtask');
  await form.getByLabel(/^Client secret/).fill('s3cr3t');
  await form.getByRole('button', { name: 'Save the provider' }).click();
  await form.waitFor({ state: 'detached' });
  assert.equal(sent.length, 1);
  assert.equal(sent[0].provisioning, 'ANY');
  assert.deepEqual(sent[0].allowed_directories, ['72f988bf-86f1-41af-91ab-2d7cd011db47']);
  assert.deepEqual(sent[0].allowed_email_domains, []);
});

// #1464: an issuer that also serves personal accounts is refused Anyone at the save
// (identity_provider.provisioning_personal), so the form does not offer it there (P-05); chosen
// before the issuer was typed, it stays chosen and the form says why, rather than moving silently.
test('chromium: Anyone is not offered on an issuer that serves personal accounts', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, 'en');
  t.after(close);

  await page.goto(`${served.origin}/administration/identity-provider`);
  await page.getByRole('button', { name: 'Add a provider' }).click();
  const form = page.locator('form.panel');
  await form.getByLabel('Which provider').selectOption('MICROSOFT');
  const admission = form.getByRole('combobox').nth(1);
  const issuer = form.getByLabel(/^Issuer/);
  const anyone = admission.locator('option[value="ANY"]');

  await issuer.fill('https://login.microsoftonline.com/organizations/v2.0');
  assert.equal(await anyone.count(), 1, 'Anyone is not offered on a shared organisations endpoint');

  await admission.selectOption('INVITED_ONLY');
  await issuer.fill('https://login.microsoftonline.com/consumers/v2.0');
  assert.equal(await anyone.count(), 0, 'Anyone is offered where the server refuses it');

  await issuer.fill('https://login.microsoftonline.com/organizations/v2.0');
  await admission.selectOption('ANY');
  await issuer.fill('https://login.microsoftonline.com/common/v2.0');
  assert.equal(await admission.inputValue(), 'ANY', 'the chosen mode was moved for the person');
  assert.match(await form.innerText(), /also serves personal accounts/);
});

// #1480: the personal accounts' directory in the list is refused by the save wherever the mode
// creates accounts (identity_provider.directory_personal), so the form says it on the field as it is
// typed and *Save* waits for it (P-05) - in any case and spacing, as the server compares it - while
// under *Only people invited here* the list only narrows and may name it.
test('chromium: the personal directory is refused in the list before the save, except where the list only narrows', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser, 'en');
  t.after(close);

  await page.goto(`${served.origin}/administration/identity-provider`);
  await page.getByRole('button', { name: 'Add a provider' }).click();
  const form = page.locator('form.panel');
  await form.getByLabel('Which provider').selectOption('MICROSOFT');
  const admission = form.getByRole('combobox').nth(1);
  const directories = form.getByLabel(/^Microsoft directories that may sign in/);
  const save = form.getByRole('button', { name: 'Save the provider' });
  const refusal = /is where this provider's personal accounts live/;

  await admission.selectOption('DOMAINS');
  await form.getByLabel(/^Issuer/).fill('https://login.microsoftonline.com/72f988bf-86f1-41af-91ab-2d7cd011db47/v2.0');
  await form.getByLabel(/^Client id/).fill('hubtask');
  await form.getByLabel(/^Client secret/).fill('s3cr3t');
  await directories.fill('72f988bf-86f1-41af-91ab-2d7cd011db47\n  9188040D-6C67-4C5B-B112-36A304B66DAD ');
  await form.getByText(refusal).first().waitFor({ timeout: 5_000 });
  assert.equal(await save.isDisabled(), true, 'Save is offered for a list the server refuses');
  await directories.press('Enter');
  assert.equal(sent.length, 0, 'the refused list was sent');

  // Only invited people: the list narrows, and naming it is permitted.
  await admission.selectOption('INVITED_ONLY');
  assert.equal(await form.getByText(refusal).count(), 0, 'the sentence stands where the server accepts the list');
  assert.equal(await save.isDisabled(), false);

  // Back to by organisation: said again.
  await admission.selectOption('DOMAINS');
  await form.getByText(refusal).first().waitFor({ timeout: 5_000 });

  // Taken out, the save goes.
  await directories.fill('72f988bf-86f1-41af-91ab-2d7cd011db47');
  assert.equal(await form.getByText(refusal).count(), 0);
  await save.click();
  await form.waitFor({ state: 'detached' });
  assert.deepEqual(sent[0]?.allowed_directories, ['72f988bf-86f1-41af-91ab-2d7cd011db47']);
});
