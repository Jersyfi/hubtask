// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the generated targets have to keep being true, whatever changes in tokens.json.
//
// These are not tests of the values - the values are the source, and a test that repeats them is
// a second place they live. They are tests of the properties ADR-0029 rests on: that the Go file
// carries names and never a colour, that the three targets describe the same ten labels, and that
// both theme modes offer the same vocabulary.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const repositoryRoot = path.resolve(packageRoot, '..', '..');
const read = (...p) => fs.readFileSync(path.join(...p), 'utf8');

const source = JSON.parse(read(packageRoot, 'tokens', 'tokens.json'));
const goFile = path.join(repositoryRoot, 'core', 'domain', 'model', 'shared', 'LabelTokens.go');

test('the Go artefact carries no colour value', () => {
  const go = read(goFile);
  assert.equal(go.match(/#[0-9a-fA-F]{3,8}\b/g), null, 'a hex value reached the core');
  assert.equal(go.match(/\brgba?\(/g), null, 'an rgb() value reached the core');
});

test('the Go artefact is marked as generated', () => {
  assert.match(read(goFile), /^\/\/ Code generated .* DO NOT EDIT\.$/m);
});

test('the Go artefact lists exactly the label tokens the source declares', () => {
  const declared = Object.keys(source.semantic.light.label);
  const generated = [...read(goFile).matchAll(/LabelToken\w+\s+LabelToken = "([a-z]+)"/g)].map((m) => m[1]);
  assert.deepEqual(generated, declared);
});

test('both modes declare the same vocabulary', () => {
  const names = (mode) => {
    const out = [];
    const walk = (node, prefix) => {
      for (const [key, value] of Object.entries(node)) {
        if (key.startsWith('$')) continue;
        if (value && typeof value === 'object' && '$value' in value) out.push([...prefix, key].join('.'));
        else walk(value, [...prefix, key]);
      }
    };
    walk(source.semantic[mode], []);
    return out.sort();
  };
  assert.deepEqual(names('light'), names('dark'));
});

test('the CSS declares primitives on :root and each mode on its own selector', () => {
  const css = read(packageRoot, 'dist', 'tokens.css');
  assert.match(css, /^:root \{$/m);
  assert.match(css, /^\[data-theme="light"\] \{$/m);
  assert.match(css, /^\[data-theme="dark"\] \{$/m);
});

test('the TypeScript target hands out custom properties, not colours', () => {
  const ts = read(packageRoot, 'dist', 'tokens.ts');
  const tokensBlock = ts.slice(ts.indexOf('export const tokens'), ts.indexOf('export const primitive'));
  assert.equal(tokensBlock.match(/#[0-9a-fA-F]{6}\b/g), null, 'a literal colour is on offer to components');
  assert.match(tokensBlock, /var\(--accent-primary-hover\)/);
});

// The two roots F2-01 added. They are mode-independent of the theme, which is exactly the property
// worth testing: a role that quietly acquired a light and a dark value would be a role two themes
// could disagree about, and that is what putting them under `semantic` would have allowed.

test('both density modes declare the same vocabulary', () => {
  const names = (mode) => {
    const out = [];
    const walk = (node, prefix) => {
      for (const [key, value] of Object.entries(node)) {
        if (key.startsWith('$')) continue;
        if (value && typeof value === 'object' && '$value' in value) out.push([...prefix, key].join('.'));
        else walk(value, [...prefix, key]);
      }
    };
    walk(source.density[mode], []);
    return out.sort();
  };
  assert.deepEqual(names('comfortable'), names('compact'));
});

test('every motion role names both a duration and an easing', () => {
  for (const [role, value] of Object.entries(source.motion)) {
    if (role.startsWith('$')) continue;
    assert.ok(value.duration?.$value, `motion.${role} has no duration`);
    assert.ok(value.easing?.$value, `motion.${role} has no easing`);
  }
});

// SC 2.5.8 asks for 24x24 CSS px, and `compact` sits exactly on it. A step below would be a
// conformance failure rather than a preference, so it fails here rather than in an audit.
test('no density mode puts a control below the minimum target size', () => {
  const space = source.primitive.space;
  const px = (reference) => {
    const step = /^\{primitive\.space\.([^}]+)\}$/.exec(reference)?.[1];
    assert.ok(step, `a density minimum is not a space reference: ${reference}`);
    return Number.parseFloat(space[step].$value);
  };
  for (const [mode, values] of Object.entries(source.density)) {
    if (mode.startsWith('$')) continue;
    for (const [size, control] of Object.entries(values.control)) {
      assert.ok(px(control.min.$value) >= 24, `density.${mode}.control.${size} is below 24px`);
    }
  }
});

test('the CSS gives density a default on :root and the theme none', () => {
  const css = read(packageRoot, 'dist', 'tokens.css');
  assert.match(css, /^:root,\n\[data-density="comfortable"\] \{$/m);
  assert.match(css, /^\[data-density="compact"\] \{$/m);
  // The asymmetry is deliberate (formats.js): no theme is right in the absence of a choice, and
  // `comfortable` is. A `:root` fallback appearing for the theme would be a regression.
  assert.doesNotMatch(css, /^:root,\n\[data-theme=/m);
});

test('no component writes a raw duration or easing where a role exists', () => {
  const componentDir = path.join(packageRoot, 'src');
  const offenders = [];
  for (const file of fs.readdirSync(componentDir).filter((f) => f.endsWith('.svelte'))) {
    const body = read(componentDir, file);
    // `--dur-instant` is rule 6's floor rather than a role: it is the absence of movement, and a
    // role for "no movement" would be a pair whose easing can never apply.
    for (const match of body.matchAll(/var\(--(?:dur|ease)-(?!instant\b)[a-z]+\)/g)) {
      offenders.push(`${file}: ${match[0]}`);
    }
  }
  assert.deepEqual(offenders, []);
});
