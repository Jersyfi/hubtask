// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The hub overview has one surface (design-system.md §4, the surfaces and levels): its tools' tiles
// are widgets, and the collections' cards and *What happened* beside them stand on the same Card, so
// neither draws a surface of its own - the owner found them on another background in dark mode.

import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const source = (name: string) => fs.readFileSync(path.join(here, name), 'utf8');

test('the collections and What happened stand on the overview tiles\' Card, with no surface of their own', () => {
  for (const name of ['CollectionCards.svelte', 'HubActivity.svelte']) {
    const text = source(name);
    assert.match(text, /<Card element=/u, `${name} does not stand on Card`);
    const style = text.slice(text.indexOf('<style>'));
    assert.doesNotMatch(style, /bg-raised|bg-surface\)|box-shadow|border-radius: var\(--r-md\)/u, `${name} draws a surface beside Card's`);
  }
});
