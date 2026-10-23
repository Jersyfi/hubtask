// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A level's words (UC-WRK-23 check 6): the name a person gave it wherever the level is named, the
// catalogue's word where none was given, and a given name never run through a type's grammar.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { StructureSettings } from '@hubtask/sync-engine';

import german from '../../../../../locales/de.json' with { type: 'json' };

import { SOURCE } from '../i18n/catalogue.ts';
import { createMessages } from '../i18n/messages.ts';
import { levelName, levelRefusal, levelWords } from './levels.ts';

const english = createMessages({ locale: 'en' });
const deutsch = createMessages({ locale: 'de', catalogues: [german, SOURCE] });
const t = english.t;

const settings = (levels: Record<string, string | null>): StructureSettings =>
  ({ levels: Object.entries(levels).map(([type, name]) => ({ type, name })) }) as unknown as StructureSettings;

const NAMED = settings({ TASK: 'item', WORK_PACKAGE: null, ACTIVITY: '  Step  ' });

test('the given name, as written; none for an unnamed, blank or unknown level', () => {
  assert.equal(levelName(NAMED, 'TASK'), 'item');
  assert.equal(levelName(NAMED, 'ACTIVITY'), 'Step');
  assert.equal(levelName(NAMED, 'WORK_PACKAGE'), undefined);
  assert.equal(levelName(settings({ TASK: '   ' }), 'TASK'), undefined);
  assert.equal(levelName(NAMED, 'MILESTONE'), undefined);
  assert.equal(levelName(undefined, 'TASK'), undefined);
});

test('a named level is said by its name everywhere it is named', () => {
  const words = levelWords(NAMED, 'TASK', t);
  assert.equal(words.name, 'item');
  assert.equal(words.label, 'item');
  assert.equal(words.add, 'Add item');
  assert.equal(words.addInside('Kitchen'), 'Add item inside Kitchen');
  assert.equal(words.newIn('Groceries'), 'New item in Groceries');
});

test('without a name, the translated default', () => {
  for (const structure of [NAMED, undefined]) {
    const words = levelWords(structure, 'WORK_PACKAGE', t);
    assert.equal(words.name, undefined);
    assert.equal(words.label, 'Work package');
    assert.equal(words.add, 'Add a work package');
    assert.equal(words.addInside('Kitchen'), 'Add a work package inside Kitchen');
    assert.equal(words.newIn('Groceries'), 'New work package in Groceries');
  }
  const german = levelWords(undefined, 'TASK', deutsch.t);
  assert.equal(german.label, 'Aufgabe');
  assert.equal(german.add, 'Aufgabe hinzufügen');
});

test('a name in another language takes that language’s sentence, never a type’s article', () => {
  const words = levelWords(settings({ TASK: 'Zutat' }), 'TASK', deutsch.t);
  assert.equal(words.label, 'Zutat');
  assert.equal(words.add, 'Zutat hinzufügen');
  assert.equal(words.addInside('Küche'), 'Zutat in Küche hinzufügen');
});

test('a refusal names its levels by their names, and stands as it is where none is named', () => {
  const invalid = { code: 'items.parent_type_invalid', params: { item_type: 'TASK', parent_type: 'WORK_PACKAGE' } };
  assert.equal(levelRefusal(NAMED, invalid, t), '“item” cannot sit in “Work package”.');
  assert.equal(levelRefusal(undefined, invalid, t), 'A task cannot sit in a work package.');
  assert.equal(
    levelRefusal(NAMED, { code: 'items.level_hidden', params: { item_type: 'ACTIVITY' } }, t),
    'This collection does not show “Step” here. What is stored stays; offer the level in the collection\'s structure to change it.',
  );
  assert.equal(
    levelRefusal(NAMED, { code: 'items.capability_not_supported', params: { item_type: 'TASK', capability: 'DUE_DATE' } }, t),
    '“item” has no due date.',
  );
  assert.equal(
    levelRefusal(NAMED, { code: 'items.depth_exceeded', params: { item_type: 'TASK', maximum: '2' } }, t),
    '“item” cannot sit that deep. At most 2 levels are allowed here.',
  );
  // Only the parent named: both levels by their labels, in the named sentence.
  assert.equal(
    levelRefusal(settings({ TASK: 'Chapter' }), { code: 'items.parent_type_invalid', params: { item_type: 'ACTIVITY', parent_type: 'TASK' } }, t),
    '“Activity” cannot sit in “Chapter”.',
  );
  // A refusal that names no level reads as it is.
  assert.equal(levelRefusal(NAMED, { code: 'app.move.moving' }, t), t('app.move.moving'));
});

test('every named sentence exists in every locale the app ships', () => {
  for (const messages of [english, deutsch]) {
    for (const code of ['add', 'add_inside', 'new_in', 'capability_not_supported', 'capability_hidden', 'level_hidden', 'parent_item_required', 'parent_type_invalid', 'depth_exceeded', 'type_unsupported']) {
      const sentence = messages.t(`app.level.${code}`, { level: 'item', parent: 'box', title: 'Kitchen', name: 'Groceries', capability: 'NOTES', maximum: '1' });
      assert.ok(sentence.includes('item'), `${messages.locale}: app.level.${code} reads "${sentence}"`);
      assert.doesNotMatch(sentence, /app\.level|\{/, `${messages.locale}: app.level.${code} reads "${sentence}"`);
    }
  }
});
