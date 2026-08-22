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
