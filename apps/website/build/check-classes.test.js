// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Fails when a class the site styles is also a class a design-system component puts in its markup.
//
// src/site.css is a global stylesheet and the components carry their own class names. Svelte's
// scoping does not keep them apart: it adds a hash class to the *component's* selectors, and a
// global `.row` in this file still matches the element underneath. So a generic name here silently
// restyles a component's internals.
//
// It is not a hypothetical. The site styled a feature row with block padding and a bottom border,
// and `Checkbox`, `ListRow`, `Radio`, `SideNav`, `DueDateControl` and `ReminderEditor` each have an
// internal `.row` - so every checkbox in the two product specimens was inflated to three times its
// height and given a rule under it. The illustration of the product stopped looking like the
// product, and nothing failed.
//
// The check runs from both directions, because either side can cause it: a generic name added
// here, or a new component that happens to use one of ours.

import assert from 'node:assert/strict';
import test from 'node:test';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const site = path.join(here, '..', 'src');
const components = path.join(here, '..', '..', '..', 'packages', 'design-system', 'src');

/** Every class `site.css` writes a rule for, comments stripped so a prose `.row` does not count. */
function classesStyledBySite() {
  const css = fs
    .readFileSync(path.join(site, 'site.css'), 'utf8')
    .replace(/\/\*[\s\S]*?\*\//g, '');
  return new Set([...css.matchAll(/\.([a-zA-Z][\w-]*)/g)].map((m) => m[1]));
}

/** Every class the component package puts on an element, from a literal attribute or `class:`. */
function classesUsedByComponents() {
  const found = new Set();
  for (const entry of fs.readdirSync(components)) {
    if (!entry.endsWith('.svelte')) continue;
    const src = fs.readFileSync(path.join(components, entry), 'utf8');
    for (const [, value] of src.matchAll(/class="([^"{]*)"/g)) {
      for (const name of value.split(/\s+/).filter(Boolean)) found.add(name);
    }
    for (const [, name] of src.matchAll(/class:([\w-]+)/g)) found.add(name);
  }
  return found;
}

test('no class the site styles is also a class a component puts in its markup', () => {
  const mine = classesStyledBySite();
  const theirs = classesUsedByComponents();
  assert.ok(mine.size > 40, 'expected site.css to style a good number of classes');
  assert.ok(theirs.size > 40, 'expected to find the component classes - has the package moved?');

  const collisions = [...mine].filter((name) => theirs.has(name)).sort();
  assert.deepEqual(
    collisions,
    [],
    'these class names are styled by src/site.css and are also used inside design-system '
      + `components, so the site is restyling their internals: ${collisions.join(', ')}. `
      + 'Prefix the site\'s own with `site-`.',
  );
});
