// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { test } from 'node:test';

import { IMPLEMENTATION, IN_WEB_APP, LEFT_OUT_AREAS, chooseTarget, redirect } from './target.ts';

test('a build chooses its target by its mode, and every other mode is the browser', () => {
  const cases: Array<[string | undefined, string]> = [
    ['desktop', 'desktop'],
    ['mobile', 'mobile'],
    ['production', 'browser'],
    ['development', 'browser'],
    ['test', 'browser'],
    ['Desktop', 'browser'],
    [undefined, 'browser'],
  ];
  for (const [mode, want] of cases) assert.equal(chooseTarget(mode), want, `mode ${String(mode)}`);
});

test('only the seam\'s own import of the browser implementation is redirected', () => {
  const index = '/repo/apps/webapp/src/lib/platform/index.ts';
  assert.equal(redirect('./browser.ts', index, 'desktop'), './desktop.ts');
  assert.equal(redirect('./browser.ts', 'C:\\repo\\apps\\webapp\\src\\lib\\platform\\index.ts', 'desktop'), './desktop.ts');
  assert.equal(redirect('./browser.ts', index, 'browser'), undefined, 'the browser build is left as written');
  assert.equal(redirect('./browser.ts', '/repo/apps/webapp/src/lib/other/index.ts', 'desktop'), undefined);
  assert.equal(redirect('./tokenStore.ts', index, 'desktop'), undefined);
  assert.equal(redirect('./browser.ts', undefined, 'desktop'), undefined);
  assert.equal(redirect('./browser.ts', index, 'mobile'), './mobile.ts');
});

test('the Android build puts the page that names the web app where the two areas would be (UC-SYN-07 check 7)', () => {
  const app = '/repo/apps/webapp/src/App.svelte';
  for (const area of LEFT_OUT_AREAS) {
    assert.equal(redirect(area, app, 'mobile'), IN_WEB_APP, area);
    assert.equal(redirect(area, app, 'desktop'), undefined, `the desktop keeps ${area}`);
    assert.equal(redirect(area, app, 'browser'), undefined, `the browser keeps ${area}`);
  }
  assert.equal(redirect('./views/TodayView.svelte', app, 'mobile'), undefined);
  assert.equal(redirect(LEFT_OUT_AREAS[0] ?? '', '/repo/apps/webapp/src/views/Other.svelte', 'mobile'), undefined);
  const root = readFileSync(new URL('../../App.svelte', import.meta.url), 'utf8');
  for (const area of [...LEFT_OUT_AREAS, IN_WEB_APP]) {
    assert.ok(existsSync(new URL(`../../${area}`, import.meta.url)), `${area} exists`);
  }
  for (const area of LEFT_OUT_AREAS) assert.ok(root.includes(`from '${area}';`), `App.svelte imports ${area} as written`);
});

test('every target\'s implementation exists, and the seam imports the browser\'s as written', () => {
  for (const file of Object.values(IMPLEMENTATION)) {
    assert.ok(existsSync(new URL(file, import.meta.url)), `${file} exists`);
  }
  const index = readFileSync(new URL('./index.ts', import.meta.url), 'utf8');
  assert.match(index, /from '\.\/browser\.ts';/, 'index.ts names the browser implementation, which the build redirects');
});
