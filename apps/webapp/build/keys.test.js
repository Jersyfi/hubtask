// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UX-5: no machine key reaches a person on the setup screens - a type, a role, a link type or an
// asset class is read by its name, and its key is asked for only behind *Advanced* when it is
// made. Read from the source, because a key beside a name is what a new list copies from its
// neighbour.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const SRC = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');
const SETUP = ['views/EntryTypesView.svelte', 'views/RolesView.svelte', 'views/RoleView.svelte', 'views/LinkTypesView.svelte', 'views/CmdbView.svelte'];

test('no setup list draws an object\'s key beside its name', () => {
  const found = [];
  for (const file of SETUP) {
    const source = fs.readFileSync(path.join(SRC, file), 'utf8');
    // `{x.key}` as text: between a tag's `>` and the next `<`, or opening a line of text.
    for (const match of source.matchAll(/(>|^\s*)\{(type|role|each)\.key\}/gm)) {
      // An object without a name of its own is named by its key, and says so where it is drawn.
      if (/data-key-is-name>$/.test(source.slice(0, match.index + 1))) continue;
      found.push(`${file}:${source.slice(0, match.index).split('\n').length}`);
    }
  }
  assert.deepEqual(found, [], 'name the object by its name; ask its key behind Advanced');
});

test('a setup screen asks the key it makes only behind Advanced', () => {
  // A role is defined in its dialog (page-system §5.1), opened from *Roles*.
  for (const file of ['views/EntryTypesView.svelte', 'lib/people/RoleDialog.svelte']) {
    const source = fs.readFileSync(path.join(SRC, file), 'utf8');
    const at = source.search(/label=\{t\('app\.(entry_types|roles)\.key'\)\}/);
    assert.ok(at > 0, `${file} asks no key`);
    const before = source.slice(0, at);
    assert.ok(before.lastIndexOf('<Disclosure') > before.lastIndexOf('</Disclosure>'), `${file} asks its key outside a Disclosure`);
  }
});

test('a form\'s section is labelled by its place, not by its key', () => {
  const source = fs.readFileSync(path.join(SRC, 'lib/entries/EntryFormEditor.svelte'), 'utf8');
  assert.doesNotMatch(source, /section_name', \{ key:/);
});
