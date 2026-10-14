// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A message that names a type of entry or of container picks the word itself: the server and this
// client fill it with the manifest's identifier, and a reader reads a sentence (rule 8). A type this
// client was built without still reads as one.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import german from '../../../../../locales/de.json' with { type: 'json' };

import { SOURCE } from './catalogue.ts';
import { createMessages } from './messages.ts';

const ITEM_TYPES = ['TASK', 'WORK_PACKAGE', 'ACTIVITY', 'MILESTONE'];
const CONTAINER_TYPES = ['HUB', 'COLLECTION', 'TENANT', 'FOLDER'];
const identifier = /\b[A-Z][A-Z_]+\b/;
/** A type's name with its capital kept in the middle of a sentence: "Add Work package". */
const capitalMidSentence = /\b(?:a|an|Add|in|under|to) (?:Task|Work package|Activity)\b/;

/** Every message whose parameters carry a type's identifier, with the parameters to fill. */
const MESSAGES: readonly { code: string; params: (type: string, parent: string) => Record<string, string> }[] = [
  { code: 'items.capability_not_supported', params: (type) => ({ item_type: type, capability: 'LABELS' }) },
  { code: 'items.parent_item_required', params: (type) => ({ item_type: type }) },
  { code: 'items.type_unsupported', params: (type) => ({ item_type: type }) },
  { code: 'items.parent_type_invalid', params: (type, parent) => ({ item_type: type, parent_type: parent }) },
  { code: 'items.depth_exceeded', params: (type) => ({ item_type: type, maximum: '3' }) },
  { code: 'fields.applies_to_unsupported', params: (type) => ({ type }) },
  { code: 'fields.not_for_this_type', params: (type) => ({ key: 'budget', type }) },
  { code: 'templates.node_not_permitted', params: (type, parent) => ({ item_type: type, parent_type: parent }) },
  { code: 'templates.root_not_permitted', params: (type) => ({ item_type: type }) },
  { code: 'app.labels.unsupported', params: (type) => ({ item_type: type }) },
  { code: 'app.entries.add_typed', params: (type) => ({ type }) },
  { code: 'app.entries.add_typed_inside', params: (type) => ({ type, title: 'Kitchen' }) },
  { code: 'app.item.type_name', params: (type) => ({ type }) },
];

for (const locale of ['en', 'de']) {
  test(`${locale}: a message about a type of entry names it in words`, () => {
    const messages = createMessages({ locale, catalogues: locale === 'de' ? [german, SOURCE] : [SOURCE] });
    for (const { code, params } of MESSAGES) {
      for (const type of ITEM_TYPES) {
        for (const parent of ITEM_TYPES) {
          const sentence = messages.t(code, params(type, parent));
          assert.doesNotMatch(sentence, identifier, `${code} for ${type} in ${parent} reads "${sentence}"`);
          if (locale === 'en') assert.doesNotMatch(sentence, capitalMidSentence, `${code} for ${type} reads "${sentence}"`);
        }
      }
    }
  });
}

test('a message about a type of container names it in words', () => {
  const messages = createMessages({ locale: 'en' });
  for (const type of CONTAINER_TYPES) {
    for (const parent of CONTAINER_TYPES) {
      const sentence = messages.t('containers.parent_type_invalid', { type, parent_type: parent });
      assert.doesNotMatch(sentence, identifier, `${type} in ${parent} reads "${sentence}"`);
    }
  }
  assert.equal(messages.t('containers.parent_type_invalid', { type: 'HUB', parent_type: 'COLLECTION' }), 'A hub cannot sit in a collection.');
});

test('the words read as sentences', () => {
  const messages = createMessages({ locale: 'en' });
  assert.equal(messages.t('items.parent_type_invalid', { item_type: 'TASK', parent_type: 'ACTIVITY' }), 'A task cannot sit in an activity.');
  assert.equal(messages.t('items.depth_exceeded', { item_type: 'ACTIVITY', maximum: '1' }), 'An activity cannot sit that deep. At most 1 level is allowed here.');
  assert.equal(messages.t('items.type_unsupported', { item_type: 'WORK_PACKAGE' }), 'This workspace does not offer work packages.');
  assert.equal(messages.t('app.entries.add_typed', { type: 'WORK_PACKAGE' }), 'Add a work package');
});
