// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Svelte 5 refuses a `bind:` whose initial value is `undefined` when the prop has a default
// (`props_invalid_value`), and it refuses it during mount, which takes the whole screen with it.
// `Select`'s `value` has a default, so every `bind:value` on one has to start from a string. The
// mistake has been made twice - BackupView (issue 535), RunsView (issue 546) - and neither the type checker
// nor the build sees it: `value?: string` accepts `undefined` in TypeScript, and the error is a
// runtime one. So the source is read instead.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const SRC = path.join(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');

/** Every variable a `<Select … bind:value={name}>` binds, by file. */
export function selectBindings(source) {
  const names = [];
  for (const tag of source.matchAll(/<Select\b[^>]*?bind:value=\{(\w+)\}/gs)) names.push(tag[1]);
  return names;
}

/** The initialiser of `let name = $state…(…)`, or undefined when the file does not declare one. */
export function stateInitialiser(source, name) {
  const declaration = source.match(new RegExp(`let ${name} = \\$state(?:<[^>]*>)?\\((.*?)\\);`, 's'));
  return declaration?.[1];
}

export function findings(file, source) {
  const found = [];
  for (const name of selectBindings(source)) {
    const initial = stateInitialiser(source, name);
    if (initial === undefined) continue; // a prop or a store field: not this check's business
    if (initial.trim() === '' || initial.trim() === 'undefined') {
      found.push(`${file}: <Select bind:value={${name}}> starts from undefined; give it $state('')`);
    }
  }
  return found;
}

function* svelteFiles(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) yield* svelteFiles(full);
    else if (entry.name.endsWith('.svelte')) yield full;
  }
}

test('every Select bind:value starts from a string', () => {
  const all = [];
  for (const file of svelteFiles(SRC)) {
    all.push(...findings(path.relative(SRC, file), fs.readFileSync(file, 'utf8')));
  }
  assert.deepEqual(all, []);
});

test('a planted undefined is found', () => {
  const planted = `<script>let picked = $state<string | undefined>(undefined);</script>
<Select label="x" bind:value={picked} options={[]} />`;
  assert.equal(findings('planted.svelte', planted).length, 1);
  assert.deepEqual(findings('fine.svelte', planted.replace('<string | undefined>(undefined)', "('')")), []);
});
