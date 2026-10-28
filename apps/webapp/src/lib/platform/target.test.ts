// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { test } from 'node:test';

import { IMPLEMENTATION, chooseTarget, redirect } from './target.ts';

test('a build chooses its target by its mode, and every other mode is the browser', () => {
  const cases: Array<[string | undefined, string]> = [
    ['desktop', 'desktop'],
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
});

test('every target\'s implementation exists, and the seam imports the browser\'s as written', () => {
  for (const file of Object.values(IMPLEMENTATION)) {
    assert.ok(existsSync(new URL(file, import.meta.url)), `${file} exists`);
  }
  const index = readFileSync(new URL('./index.ts', import.meta.url), 'utf8');
  assert.match(index, /from '\.\/browser\.ts';/, 'index.ts names the browser implementation, which the build redirects');
});
