// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The gate that makes a hand-written ICU subset safe.
//
// `format.ts` refuses syntax it does not implement rather than guessing at it. That refusal is
// only worth anything if somebody hears it, and the moment to hear it is when a message is written
// into the catalogue - not when a reader of Polish sees the wrong plural. So every message in
// `locales/en.json` is parsed here, and a construct the client cannot render turns this red.
//
// It is the client's counterpart to `infrastructure/i18n/Catalogue_test.go`, which refuses a
// plural on the Go side because the Go renderer implements only simple arguments. The two gates
// draw the line in different places on purpose: the server renders a handful of messages for
// email, and the client renders all of them.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { SOURCE, SOURCE_LOCALE, patternFor } from './catalogue.ts';
import { format, parse } from './format.ts';

test('the catalogue is the one at the repository root, and it is not empty', () => {
  assert.ok(Object.keys(SOURCE).length > 500, 'the catalogue looks truncated');
  assert.equal(SOURCE['errors.unauthenticated'], 'Please sign in.');
});

test('no note to the translators is offered as a message', () => {
  for (const code of Object.keys(SOURCE)) {
    assert.ok(!code.startsWith('_'), `${code} is metadata and would be rendered as a sentence`);
  }
});

test('every message in the catalogue is one this renderer can render', () => {
  const failures: string[] = [];
  for (const [code, pattern] of Object.entries(SOURCE)) {
    try {
      parse(pattern);
    } catch (error) {
      failures.push(`${code}: ${(error as Error).message}`);
    }
  }
  assert.deepEqual(failures, [], 'add the construct to format.ts, or write the message another way');
});

test('the errors.* codes render as the sentences the catalogue gives them', () => {
  assert.equal(format(SOURCE['errors.forbidden']!), 'You do not have permission for this action.');
  assert.equal(
    format(SOURCE['errors.internal']!, { request_id: '01J9Z2' }),
    'Something went wrong on our side. Reference: 01J9Z2',
  );
});

test('a missing translation falls back to the source language, never to a key', () => {
  const german: Record<string, string> = { 'errors.forbidden': 'Dazu fehlt dir die Berechtigung.' };
  assert.equal(patternFor('errors.forbidden', [german, SOURCE]), 'Dazu fehlt dir die Berechtigung.');
  assert.equal(patternFor('errors.not_found', [german, SOURCE]), 'This entry does not exist.');
  assert.equal(patternFor('errors.invented', [german, SOURCE]), undefined);
  assert.equal(SOURCE_LOCALE, 'en');
});

// Every catalogue, not only the source (F5-07). A translator's ICU error fails the build here the
// way M-02's gate fails it on the server; and the `_comment` prefix is skipped in every file, the
// way the loader skips it, so a note to the translators is never a message anywhere.
const LOCALES = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..', '..', '..', '..', 'locales');

test('every message of every catalogue is one this renderer can render', () => {
  const files = fs.readdirSync(LOCALES).filter((name) => name.endsWith('.json'));
  assert.ok(files.includes('en.json') && files.length >= 2, `the catalogues are ${files.join(', ')}`);
  const failures: string[] = [];
  for (const file of files) {
    const entries = JSON.parse(fs.readFileSync(path.join(LOCALES, file), 'utf8')) as Record<string, string>;
    for (const [code, pattern] of Object.entries(entries)) {
      if (code.startsWith('_')) continue;
      try {
        parse(pattern);
      } catch (error) {
        failures.push(`${file} ${code}: ${(error as Error).message}`);
      }
    }
  }
  assert.deepEqual(failures, [], 'a catalogue carries a construct the client cannot render');
});

test('a code the second catalogue lacks falls back to the source, and one it has does not', () => {
  const german = JSON.parse(fs.readFileSync(path.join(LOCALES, 'de.json'), 'utf8')) as Record<string, string>;
  const withoutNotes = Object.fromEntries(Object.entries(german).filter(([code]) => !code.startsWith('_')));
  const translated = Object.keys(withoutNotes).find((code) => SOURCE[code] !== undefined);
  assert.ok(translated, 'de.json translates nothing the source has');
  assert.equal(patternFor(translated, [withoutNotes, SOURCE]), withoutNotes[translated]);
  const untranslated = Object.keys(SOURCE).find((code) => withoutNotes[code] === undefined);
  assert.ok(untranslated, 'de.json is complete, so this test has nothing to fall back on');
  assert.equal(patternFor(untranslated, [withoutNotes, SOURCE]), SOURCE[untranslated]);
});
