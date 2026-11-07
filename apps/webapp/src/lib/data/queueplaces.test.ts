// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { ENTITY_PLACES, QUEUEABLE_ENTITIES, draftFields, fieldRefusals } from './queueplaces.ts';

const catalogue = (name: string): Record<string, string> =>
  JSON.parse(readFileSync(new URL(`../../../../../locales/${name}`, import.meta.url), 'utf8')) as Record<string, string>;

test('every queueable entity says what changed in each catalogue and where it is opened', () => {
  for (const name of ['en.json', 'de.json']) {
    const messages = catalogue(name);
    for (const entity of QUEUEABLE_ENTITIES) {
      for (const act of ['write', 'delete']) {
        assert.ok(messages[`app.sync.what.entity.${entity}.${act}`], `${name} has no app.sync.what.entity.${entity}.${act}`);
      }
    }
  }
  for (const entity of QUEUEABLE_ENTITIES) {
    // A link opens the entry it is drawn from; every other entity names its screen.
    if (entity !== 'entry_link') assert.ok(ENTITY_PLACES[entity].href, `${entity} opens nowhere`);
  }
});

test('a draft offers the text a refused change carried, nothing else', () => {
  assert.deepEqual(draftFields({ title: 'far too long', due_at: '2026-11-04', order_key: 3 }), { title: 'far too long' });
  assert.deepEqual(draftFields({ name: 'Incident', notes: 'x' }), { name: 'Incident', notes: 'x' });
  assert.deepEqual(draftFields(undefined), {});
  assert.deepEqual(draftFields(['title']), {});
});

test('a refusal names the draft fields it is about, read from their JSON Pointer', () => {
  const refusals = fieldRefusals([
    { path: '/title', code: 'items.title_too_long', params: { max: '500' } },
    { path: '/fields/notes', code: 'items.nested' },
    { path: '/a~1b', code: 'items.escaped' },
    { path: '', code: 'items.whole' },
  ]);
  assert.deepEqual(refusals, {
    title: { code: 'items.title_too_long', params: { max: '500' } },
    'a/b': { code: 'items.escaped', params: {} },
  });
  assert.deepEqual(fieldRefusals(undefined), {});
});
