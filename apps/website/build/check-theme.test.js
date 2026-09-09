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

// ── The shapes the colour-mode animation is written against ──────────────────
//
// The three faces animate their own parts: the sun's eight rays come out of its disc in turn, the
// moon's two star strokes twinkle in after the crescent, and the hybrid's rays follow its body.
// Those rules select by element type and by `nth-child`, which means they are written against the
// *shape* of three icons in a generated file.
//
// Lucide could reorder or reshape any of them in an upgrade. Nothing would break - the glyph would
// still draw - but a ray would animate as though it were a star, and nobody would notice for
// months. So the shapes are asserted here: an upgrade that changes one fails this test, and the
// stylesheet is corrected in the same change rather than found later.

import { BASE_ICONS } from '../../../packages/design-system/src/icons/base.ts';

/** The tag sequence of an icon, which is what an `nth-child` rule actually depends on. */
const shapeOf = (name) => (BASE_ICONS[name] ?? []).map(([tag]) => tag);

test('the sun is a disc and eight separate rays, in that order', () => {
  const shape = shapeOf('sun');
  assert.equal(shape[0], 'circle', 'site.css styles the sun disc as `svg circle`');
  assert.deepEqual(
    shape.slice(1),
    Array(8).fill('path'),
    'site.css staggers the sun rays as `path:nth-child(2)` through `(9)` - the count or the tags have changed',
  );
});

test('the moon carries its two star strokes before the crescent', () => {
  const shape = shapeOf('moon-star');
  assert.deepEqual(shape, ['path', 'path', 'path'], 'moon-star is expected to be three paths');
  const [first, second, crescent] = BASE_ICONS['moon-star'].map(([, a]) => a.d ?? '');
  assert.ok(
    first.length < crescent.length && second.length < crescent.length,
    'site.css twinkles `path:nth-child(1)` and `(2)` as the star and leaves the crescent alone; '
      + 'the crescent is no longer the third and longest path',
  );
});

test('the hybrid keeps a ray in the first, fourth and fifth position', () => {
  const shape = shapeOf('sun-moon');
  assert.equal(shape.length, 5, 'site.css delays `path:nth-child(4)` and `(5)` of sun-moon');
  assert.deepEqual(shape, Array(5).fill('path'), 'sun-moon is expected to be five paths');
});

test('every glyph the control paints is in the icon set', () => {
  for (const name of ['sun', 'moon-star', 'sun-moon']) {
    assert.ok(BASE_ICONS[name], `+layout.svelte paints <Icon name="${name}"> and the set has no such mark`);
  }
});
