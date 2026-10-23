// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { BUILT_IN } from '../data/presetChoice.ts';
import { PURPOSES, STARTER_SETS } from './starterSet.ts';

const catalogue = JSON.parse(readFileSync(new URL('../../../../../locales/en.json', import.meta.url), 'utf8')) as Record<string, string>;

test('every purpose has a hub on a preset and named collections, each name in the catalogue (UC-WRK-30 check 5)', () => {
  for (const purpose of PURPOSES) {
    const set = STARTER_SETS[purpose];
    assert.ok((BUILT_IN as readonly string[]).includes(set.hub.preset), `${purpose}'s hub has no preset`);
    assert.ok(set.collections.length > 0, `${purpose} makes no collection`);
    for (const place of [set.hub, ...set.collections]) {
      assert.ok(catalogue[place.name], `${place.name} is not in the catalogue`);
      assert.ok(place.preset === undefined || (BUILT_IN as readonly string[]).includes(place.preset));
    }
    assert.ok(catalogue[`app.starter.purpose.${purpose}`], `${purpose} is not worded`);
  }
});
