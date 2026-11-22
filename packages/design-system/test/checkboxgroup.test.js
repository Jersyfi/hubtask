// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// CheckboxGroup (design-system.md §4, wave 1), read without a browser: it stays a component of its
// own and draws its options through MultiSelect's ticks, so the two cannot drift apart (ADR-0110).
// That it did not move a box is measured in the browser, not here.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const sourceDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');
const source = fs.readFileSync(path.join(sourceDir, 'CheckboxGroup.svelte'), 'utf8');
const markup = source.slice(source.lastIndexOf('</script>'), source.indexOf('<style>'));

test('the options are the shared ticks, not boxes of its own', () => {
  assert.match(source, /import Ticks from '\.\/_Ticks\.svelte'/);
  assert.match(markup, /<Ticks /);
  assert.doesNotMatch(markup, /<Checkbox/, 'a row of its own would be a second arrangement to drift');
});

test('the question is still the fieldset’s legend, and the error is said once for the group', () => {
  assert.match(markup, /<fieldset class="group" data-invalid=\{error \? '' : undefined\}/);
  assert.match(markup, /<legend class="legend">\{label\}<\/legend>/);
  assert.match(markup, /\{errorId\}/, 'each box names the group’s error');
  assert.equal(markup.match(/class="message error"/g)?.length, 1);
});
