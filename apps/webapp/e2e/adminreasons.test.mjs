// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-WRK-32 check 7 for the administration and the installation: every page of both areas, for
// somebody who may change it and somebody who only reads it, at a phone's, a tablet's and a
// desktop's width, explains an unavailable control without breaking the page:
// * no reason stands in the flow - an unavailable control's reason is its own description;
// * no save is unavailable for want of a change or of a field (check 3);
// * a read-only area says why once, never the same sentence twice on a page (check 4);
// * nothing is wider than the screen.
//
// The pages are every route `lib/routes.ts` tags with the area, so a page added there is walked
// here without anybody remembering to. That a reason moves no box is UX-03's measured claim
// (`workbench-layout.test.mjs`); what a press of an empty form does is walked per form.
//
// Against the shared fixture. Chromium; the chip's behaviour in Firefox and WebKit is
// `engines.test.mjs`'s.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ROUTES } from '../src/lib/routes.ts';
import { MANIFEST, PERMISSIONS, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

/** A route's pattern as a path to open: a parameter takes a value the fixture does not hold. */
const pathOf = (pattern) => pattern.replace(/:[a-z]+/g, 'walk');

const ADMINISTRATION = ROUTES.filter((route) => route.area === 'administration').map((route) => pathOf(route.pattern));
const INSTALLATION = ROUTES.filter((route) => route.area === 'instance').map((route) => pathOf(route.pattern));

const WIDTHS = [375, 768, 1280];

/** The words a reason used to stand in the page with; none of them may be in the flow now. */
const OLD_REASONS = [
  'Nothing has changed yet.',
  'Choose one first.',
  'Name a period first.',
  'Name a period and a target first.',
  'Tick at least one scope',
  'A name and a short name first.',
  'Look at what it would do first.',
];

async function as(browser, width, permissions, colorScheme = 'light') {
  const context = await browser.newContext({ viewport: { width, height: 900 }, colorScheme });
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: permissions } });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(`${new URL(page.url()).pathname}: ${error}`));
  return { context, page, failures };
}

/** What a page breaks of check 7, as sentences; empty where it holds. */
async function breaches(page) {
  return page.evaluate((old) => {
    const found = [];
    const shown = (element) => {
      if (element.closest('[hidden], [role="tooltip"]')) return false;
      const box = element.getBoundingClientRect();
      return box.width > 0 && box.height > 0;
    };
    // A reason in the flow: the old line under a control, or one of the old sentences.
    for (const line of document.querySelectorAll('main .reason')) {
      if (shown(line)) found.push(`a reason in the flow: ${line.textContent?.trim()}`);
    }
    const text = document.querySelector('main')?.innerText ?? '';
    for (const sentence of old) if (text.includes(sentence)) found.push(`the old sentence "${sentence}"`);
    // An unavailable control explains itself through its own description, which exists.
    for (const control of document.querySelectorAll('main [aria-disabled="true"]')) {
      if (control.closest('[inert]')) continue;
      const ids = (control.getAttribute('aria-describedby') ?? '').split(' ').filter(Boolean);
      const said = ids.map((id) => document.getElementById(id)?.textContent?.trim() ?? '').join(' ');
      if (!said) found.push(`an unavailable control without its reason: ${control.textContent?.trim() || control.getAttribute('aria-label')}`);
      if (/^save/i.test(control.textContent?.trim() ?? '')) found.push(`an unavailable save: ${control.textContent?.trim()}`);
    }
    // A read-only area says why once: no note twice on one page.
    const notes = [...document.querySelectorAll('main [role="note"]')].map((note) => note.textContent?.trim() ?? '');
    for (const [index, note] of notes.entries()) {
      if (note && notes.indexOf(note) !== index) found.push(`the same notice twice: ${note}`);
    }
    // Nothing wider than the screen.
    if (document.documentElement.scrollWidth > window.innerWidth + 1) {
      found.push(`the page is wider than the screen: ${document.documentElement.scrollWidth} > ${window.innerWidth}`);
    }
    return found;
  }, OLD_REASONS);
}

/** Opens every path in one tab and collects what each breaks. */
async function walk(page, paths) {
  const found = [];
  for (const path of paths) {
    await page.goto(`${served.origin}${path}`);
    await page.locator('main').waitFor({ timeout: 15_000 });
    // The reads the page starts arrive from the stub at once; this lets them draw.
    await page.waitForTimeout(400);
    for (const breach of await breaches(page)) found.push(`${path}: ${breach}`);
  }
  return found;
}

for (const width of WIDTHS) {
  for (const [who, permissions] of [['an owner', PERMISSIONS.OWNER], ['an auditor', PERMISSIONS.AUDITOR]]) {
    test(`chromium: ${width} px — every administration page explains without breaking, for ${who}`, async (t) => {
      const browser = await chromium.launch();
      t.after(() => browser.close());
      const { context, page, failures } = await as(browser, width, permissions);
      t.after(() => context.close());
      assert.ok(ADMINISTRATION.length >= 20, `only ${ADMINISTRATION.length} administration routes were found`);
      assert.deepEqual(await walk(page, ADMINISTRATION), []);
      assert.deepEqual(failures, []);
    });
  }

  test(`chromium: ${width} px — every installation page explains without breaking`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures } = await as(browser, width, PERMISSIONS.OWNER);
    t.after(() => context.close());
    assert.ok(INSTALLATION.length >= 10, `only ${INSTALLATION.length} installation routes were found`);
    assert.deepEqual(await walk(page, INSTALLATION), []);
    assert.deepEqual(failures, []);
  });
}

test('chromium: 768 px, dark — the administration and the installation explain without breaking', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await as(browser, 768, PERMISSIONS.AUDITOR, 'dark');
  t.after(() => context.close());
  assert.deepEqual(await walk(page, [...ADMINISTRATION, ...INSTALLATION]), []);
  assert.deepEqual(failures, []);
});
