// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Every script in its own face (UC-ID-23), in a browser: the faces a page loads are the ones its
// text needs, all from this installation, and a heading in a script the condensed cut lacks is set
// in IBM Plex Sans.
//
// The manifest here offers five more languages than the shared fixture's, and the entries and the
// collection are retitled in each one's script; the shared fixture itself is left as it is. No
// catalogue exists for these languages, so the interface renders the source language under a
// document that says `ar` or `ru` - which is what decides the faces: `lang`, `data-script` and the
// letters on the page.
//
// Chromium only: what is measured is which files the stylesheet's ranges make a browser fetch.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, COLLECTION, ITEMS, MANIFEST, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const ENTRY = ITEMS[0];

const served = await serve(DIST);
test.after(() => served.close());

const LOCALES = [
  { locale: 'en', direction: 'ltr' },
  { locale: 'de', direction: 'ltr' },
  { locale: 'ar', direction: 'rtl' },
  { locale: 'hi', direction: 'ltr' },
  { locale: 'bn', direction: 'ltr' },
  { locale: 'zh-Hans', direction: 'ltr' },
  { locale: 'ru', direction: 'ltr' },
];

/** Per language: the collection's name, the entries' titles, and the files its face is shipped as. */
const SCRIPTS = {
  ar: { collection: 'المطبخ', titles: ['اطلب البلاط', 'احجز الكهربائي', 'قِس سطح العمل', 'اختر الطلاء', 'أفرغ الغرفة'], face: 'IBM Plex Sans Arabic', file: /ibm-plex-sans-arabic-arabic-/ },
  hi: { collection: 'रसोई', titles: ['टाइलें मँगवाओ', 'बिजली मिस्त्री बुलाओ', 'काउंटर नापो', 'रंग चुनो', 'कमरा खाली करो'], face: 'IBM Plex Sans Devanagari', file: /ibm-plex-sans-devanagari-devanagari-/ },
  bn: { collection: 'রান্নাঘর', titles: ['টাইল অর্ডার করো।', 'ইলেকট্রিশিয়ান ডাকো', 'কাউন্টার মাপো', 'রং বাছো', 'ঘর খালি করো'], face: 'Noto Sans Bengali', file: /noto-sans-bengali-bengali-/,
    // The danda is encoded once, in the Devanagari block, for both scripts: Bengali text fetches the
    // Devanagari face for it.
    shares: /ibm-plex-sans-devanagari-devanagari-/ },
  'zh-Hans': { collection: '厨房', titles: ['订购瓷砖', '预约电工', '测量台面', '选择油漆', '清空房间'], face: 'IBM Plex Sans SC', file: /IBMPlexSansSC-/ },
  ru: { collection: 'Кухня', titles: ['Заказать плитку', 'Вызвать электрика', 'Измерить столешницу', 'Выбрать краску', 'Освободить комнату'], face: 'IBM Plex Sans', file: /ibm-plex-sans-cyrillic-wght/ },
};

/** Every file of a script face, as the bundle ships it. */
const SCRIPT_FACE = /(ibm-plex-sans-(arabic|hebrew|devanagari|thai)-|noto-sans-bengali-|IBMPlexSansSC-)/;

/** The fixture's answers, with the collection and the entries renamed into a script. */
function retitled(script) {
  const rename = (value) => {
    if (Array.isArray(value)) return value.map(rename);
    if (value && typeof value === 'object') {
      const copy = Object.fromEntries(Object.entries(value).map(([key, each]) => [key, rename(each)]));
      const index = ITEMS.findIndex((item) => item.id === copy.id);
      if (index >= 0 && typeof copy.title === 'string') copy.title = script.titles[index];
      if (copy.id === COLLECTION.id && typeof copy.name === 'string') copy.name = script.collection;
      return copy;
    }
    return value;
  };
  return (route) => stub({ request: () => route.request(), fulfill: (answer) => route.fulfill(answer.json === undefined ? answer : { ...answer, json: rename(answer.json) }) });
}

/** A signed-in page in a language, every request it makes recorded before the first one leaves. */
async function openIn(browser, locale, script) {
  const { context, page, failures } = await signedIn(browser, 1280, 900);
  if (script) {
    await context.unroute('**/api/v1/**');
    await context.route('**/api/v1/**', retitled(script));
  }
  // Registered after the fixture, so they answer first.
  await context.route('**/api/v1/meta/capabilities', (route) => route.fulfill({ json: { ...MANIFEST, supported_locales: LOCALES } }));
  for (const path of ['**/api/v1/accounts/me', `**/api/v1/accounts/${ACCOUNT.id}`]) {
    await context.route(path, (route) => route.fulfill({ json: { ...ACCOUNT, locale } }));
  }
  const requests = [];
  page.on('request', (request) => requests.push({ url: request.url(), type: request.resourceType() }));
  return { context, page, failures, requests, fonts: () => requests.filter((each) => each.type === 'font') };
}

/** The faces the document has loaded, by family. */
const loadedFaces = (page) => page.evaluate(async () => {
  await document.fonts.ready;
  return [...document.fonts].filter((face) => face.status === 'loaded').map((face) => face.family.replace(/["']/g, ''));
});

test('chromium: an English page loads no script face, an arrow in its history included', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures, fonts } = await openIn(browser, 'en');
  t.after(() => context.close());
  // A history step whose detail is "from → to": U+2192 is outside the Latin face's ranges, and
  // must not be what fetches a script face for an English reader.
  await context.route((url) => url.pathname.endsWith(`/api/v1/items/${ENTRY.id}/activity`), (route) => route.fulfill({ json: { data: [{
    id: '01a0e2e0-0000-7000-8000-0000000007a1', item_id: ENTRY.id, code: 'activity.item_updated',
    actor: { type: 'USER', id: ACCOUNT.id }, occurred_at: '2026-10-20T09:00:00Z',
    change_set: { title: { from: 'Order the tiles', to: ENTRY.title } },
  }], page: { next_cursor: null, has_more: false } } }));

  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('textbox', { name: 'Title' }).first().waitFor({ timeout: 15_000 });
  await page.getByRole('tablist', { name: "The entry's history" }).getByRole('tab', { name: 'History' }).click();
  await page.getByText('→').first().waitFor({ timeout: 5_000 });
  await loadedFaces(page);

  assert.equal(await page.evaluate(() => document.documentElement.dataset.script), 'Latn');
  const scriptFiles = fonts().filter((each) => SCRIPT_FACE.test(each.url));
  assert.deepEqual(scriptFiles.map((each) => each.url), [], 'an English page fetched a script face');
  assert.ok(fonts().length > 0, 'no font was requested at all - the measurement measured nothing');
  for (const each of fonts()) assert.ok(each.url.startsWith(`${served.origin}/`), `a font from elsewhere: ${each.url}`);
  assert.deepEqual(failures, []);
});

for (const [locale, script] of Object.entries(SCRIPTS)) {
  test(`chromium: ${locale} — the script's own face, from this installation, and only the parts it needs`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures, fonts } = await openIn(browser, locale, script);
    t.after(() => context.close());

    await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
    await page.getByText(script.titles[0]).first().waitFor({ timeout: 15_000 });
    await page.waitForFunction((tag) => document.documentElement.lang === tag, locale, { timeout: 5_000 });
    const loaded = await loadedFaces(page);

    // Check 1: the text is set in the face made for its script.
    assert.ok(loaded.includes(script.face), `${script.face} was not loaded: ${[...new Set(loaded)].join(', ')}`);
    assert.ok(fonts().some((each) => script.file.test(each.url)), `no file of ${script.face} was fetched`);

    // Check 2: the heading - in the display stack - is set in IBM Plex Sans, not the condensed cut,
    // which covers none of these scripts.
    const heading = page.getByRole('heading', { level: 1, name: script.collection });
    const family = await heading.evaluate((element) => getComputedStyle(element).fontFamily);
    assert.match(family, /^"?IBM Plex Sans"?,/, `the heading is set in ${family}`);
    assert.ok(loaded.includes('IBM Plex Sans'), 'IBM Plex Sans was not loaded for the heading');

    // Check 3: every font from this installation.
    for (const each of fonts()) assert.ok(each.url.startsWith(`${served.origin}/`), `a font from elsewhere: ${each.url}`);

    // Check 4: only the parts the text needs - no other script's face, and of SC's chunks only
    // those the characters on the page fall in.
    const others = fonts().filter((each) => SCRIPT_FACE.test(each.url) && !script.file.test(each.url) && !script.shares?.test(each.url));
    assert.deepEqual(others.map((each) => each.url), [], `a ${locale} page fetched another script's face`);
    if (locale === 'zh-Hans') {
      const shipped = fs.readdirSync(join(DIST, 'assets')).filter((file) => /^IBMPlexSansSC-/.test(file)).length;
      const fetched = fonts().filter((each) => script.file.test(each.url)).length;
      assert.ok(fetched > 0 && fetched < shipped / 10, `${fetched} of ${shipped} SC files fetched for a page of a few words`);
    }
    assert.deepEqual(failures, []);
  });
}
