// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { applyDocumentLocale, directionOf, fallbackChain, resolveLocale } from './locale.ts';

const supported = [
  { locale: 'en', direction: 'ltr' as const },
  { locale: 'de', direction: 'ltr' as const },
  { locale: 'ar', direction: 'rtl' as const },
];

test('a tag falls back along its own chain, not to the source language', () => {
  assert.deepEqual(fallbackChain('de-AT'), ['de-at', 'de']);
  assert.deepEqual(fallbackChain('zh-Hans-CN'), ['zh-hans-cn', 'zh-hans', 'zh']);
  assert.deepEqual(fallbackChain('en'), ['en']);
});

test('the account preference wins over the browser', () => {
  // §2's parenthesis: `Accept-Language` counts "only for anonymous/client responses". Somebody who
  // set German in their account gets German on a borrowed English laptop.
  assert.equal(resolveLocale({ account: 'de', requested: ['en-GB', 'en'] }, supported), 'de');
});

test('the browser answers before there is an account to ask', () => {
  assert.equal(resolveLocale({ requested: ['de-AT', 'en'] }, supported), 'de');
  assert.equal(resolveLocale({ requested: ['fr-CA', 'ar'] }, supported), 'ar');
});

test('the tenant and then the installation answer when nobody else can', () => {
  assert.equal(resolveLocale({ requested: ['fr'], tenant: 'de', installation: 'ar' }, supported), 'de');
  assert.equal(resolveLocale({ requested: ['fr'], installation: 'ar' }, supported), 'ar');
});

test('an installation that supports none of them still renders something', () => {
  // §3: never a key, never an empty string. The source language is the floor.
  assert.equal(resolveLocale({ account: 'fr', requested: ['ja'] }, supported), 'en');
  assert.equal(resolveLocale({}, []), 'en');
});

test('the direction comes from the manifest, not from a list written in the client', () => {
  assert.equal(directionOf('ar', supported), 'rtl');
  assert.equal(directionOf('ar-EG', supported), 'rtl');
  assert.equal(directionOf('de', supported), 'ltr');
  // An installation that has not declared the locale gets the safe answer rather than a guess.
  assert.equal(directionOf('he', supported), 'ltr');
});

test('the document says both, in one place', () => {
  const written: Record<string, string> = {};
  applyDocumentLocale({ setAttribute: (name, value) => (written[name] = value) }, 'ar-EG', 'rtl');
  assert.deepEqual(written, { lang: 'ar-EG', dir: 'rtl' });
});
