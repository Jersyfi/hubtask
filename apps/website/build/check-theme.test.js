// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The colour-mode construction in src/site.css has one way to rot, and this is the guard for it.
//
// The document is dark and the body carries the light set; two rules hand the light set back to
// the document's dark one by setting every semantic custom property the site reads to `inherit`.
// A property the site starts using and those rules do not list would keep its light value in dark
// mode - a light card on a dark page, in one place, noticed by nobody.
//
// So: read the tokens the stylesheet actually uses, read what each neutralising block covers, and
// fail on the difference. The token names come from the design system's own naming module rather
// than from a copy of the rules, because a second copy of a naming rule is the thing this whole
// package exists to prevent.

import assert from 'node:assert/strict';
import test from 'node:test';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { cssName } from '../../../packages/design-system/build/naming.js';

const here = path.dirname(fileURLToPath(import.meta.url));
const css = fs.readFileSync(path.join(here, '..', 'src', 'site.css'), 'utf8');
const tokens = JSON.parse(
  fs.readFileSync(path.join(here, '..', '..', '..', 'packages', 'design-system', 'tokens', 'tokens.json'), 'utf8'),
);

/** Every semantic custom property name the design system declares, in either mode. */
function semanticNames() {
  const names = new Set();
  const walk = (node, trail) => {
    if (node && typeof node === 'object') {
      if ('$value' in node) return names.add(`--${cssName(trail)}`);
      for (const [key, value] of Object.entries(node)) {
        if (!key.startsWith('$')) walk(value, [...trail, key]);
      }
    }
  };
  walk(tokens.semantic.light, ['semantic', 'light']);
  return names;
}

/** The declarations of one `{ … }` block, found by the selector that opens it. */
function blockFor(selector) {
  const at = css.indexOf(selector);
  assert.notEqual(at, -1, `src/site.css no longer contains the rule "${selector}" - has the colour-mode construction changed?`);
  const open = css.indexOf('{', at);
  const close = css.indexOf('}', open);
  return new Set([...css.slice(open, close).matchAll(/(--[\w-]+):\s*inherit/g)].map((m) => m[1]));
}

const SEMANTIC = semanticNames();

test('every semantic token the site uses is neutralised in both colour-mode blocks', () => {
  const used = new Set(
    [...css.matchAll(/var\((--[\w-]+)\)/g)].map((m) => m[1]).filter((name) => SEMANTIC.has(name)),
  );
  assert.ok(used.size > 20, 'expected the stylesheet to read a good number of semantic tokens');

  for (const selector of ['html:has(#mode-dark:checked) body[data-theme]', 'html:has(#mode-auto:checked) body[data-theme]']) {
    const covered = blockFor(selector);
    const missing = [...used].filter((name) => !covered.has(name)).sort();
    assert.deepEqual(
      missing,
      [],
      `these tokens are read by src/site.css but keep their light value under "${selector}", `
        + `so they would stay light in dark mode: ${missing.join(', ')}`,
    );
  }
});

test('the two colour-mode blocks cover exactly the same tokens', () => {
  const dark = [...blockFor('html:has(#mode-dark:checked) body[data-theme]')].sort();
  const auto = [...blockFor('html:has(#mode-auto:checked) body[data-theme]')].sort();
  assert.deepEqual(dark, auto, 'the explicit-dark and the follow-the-system block have drifted apart');
});

test('no semantic token is neutralised that the design system does not declare', () => {
  for (const name of blockFor('html:has(#mode-dark:checked) body[data-theme]')) {
    assert.ok(SEMANTIC.has(name), `${name} is neutralised in src/site.css but is not a semantic token`);
  }
});
