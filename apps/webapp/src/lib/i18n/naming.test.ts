// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Catalogue } from './catalogue.ts';
import { namingLocale, namingMessages } from './naming.ts';

const SUPPORTED = [
  { locale: 'en', direction: 'ltr' as const },
  { locale: 'de', direction: 'ltr' as const },
  { locale: 'fr', direction: 'ltr' as const },
];

test('a stored name follows the account, else the workspace - never the interface', () => {
  assert.equal(namingLocale({ account: 'fr', workspace: 'de' }, SUPPORTED, 'en'), 'fr');
  assert.equal(namingLocale({ account: null, workspace: 'de' }, SUPPORTED, 'en'), 'de');
  assert.equal(namingLocale({ account: '  ', workspace: 'de-AT' }, SUPPORTED, 'en'), 'de');
});

test('only while neither is known does the interface language answer', () => {
  assert.equal(namingLocale({}, SUPPORTED, 'en'), 'en');
});

test('the naming catalogue is read through the loader, the source under it', async () => {
  const catalogues: Record<string, Catalogue> = {
    en: { 'app.pages.untitled': 'Untitled page', 'app.posts.untitled': 'Untitled post' },
    de: { 'app.pages.untitled': 'Unbenannte Seite' },
  };
  const asked: string[] = [];
  const load = async (locale: string) => {
    asked.push(locale);
    return catalogues[locale];
  };
  const german = await namingMessages('de', load);
  assert.equal(german.t('app.pages.untitled'), 'Unbenannte Seite');
  assert.equal(german.t('app.posts.untitled'), 'Untitled post');
  assert.deepEqual(asked.sort(), ['de', 'en']);
});

test('a catalogue that does not arrive leaves the source', async () => {
  const load = async (locale: string) => {
    if (locale === 'de') throw new Error('offline');
    return { 'app.pages.untitled': 'Untitled page' };
  };
  assert.equal((await namingMessages('de', load)).t('app.pages.untitled'), 'Untitled page');
});
