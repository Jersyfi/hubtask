// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The two things this package must never acquire, as a gate rather than as a paragraph.
//
// F1-09's brief writes both down "because they are cheap to prevent and expensive to remove", and
// a rule that is only written down is a rule somebody breaks in good faith two years from now,
// when the reason has left the room. So it is a test.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

function sources(dir: string): { relative: string; text: string }[] {
  const out: { relative: string; text: string }[] = [];
  const walk = (at: string) => {
    for (const entry of fs.readdirSync(at, { withFileTypes: true })) {
      const joined = path.join(at, entry.name);
      if (entry.isDirectory()) {
        if (entry.name !== 'node_modules' && entry.name !== 'dist') walk(joined);
      } else if (/\.(ts|js)$/.test(entry.name)) {
        out.push({ relative: path.relative(packageRoot, joined), text: fs.readFileSync(joined, 'utf8') });
      }
    }
  };
  walk(dir);
  return out;
}

const ALL = [...sources(path.join(packageRoot, 'src')), ...sources(path.join(packageRoot, 'test'))];

test('there are sources to read', () => {
  assert.ok(ALL.length >= 5, `only ${ALL.length} files found - the walk is broken`);
});

// The engine never merges. Merging is the server's (ADR-0021, offline-sync.md §4): this package
// queues, pushes, applies what the server answers, and surfaces the conflict for the UI to render.
// A merge rule here is a bug against that decision rather than a feature - so a *symbol* that
// merges is refused, and the word in a comment saying we do not is not.
test('nothing in this package merges', () => {
  // Declarations and calls, not prose: `#merge(`, `function merge`, `merged =`, `resolveConflict`.
  const merging =
    /(?:^|[^\w'"`])(?:#|function\s+|const\s+|let\s+|async\s+)?(?:merge|merged|mergeWith|resolveConflict|lastWriteWins|reconcile)\s*(?:\(|=[^=])/;
  for (const file of ALL) {
    const found = file.text
      .split('\n')
      // A comment explaining that merging is the server's is exactly what should be here.
      .filter((line) => !/^\s*(?:\/\/|\*|\/\*)/.test(line))
      .find((line) => merging.test(line));
    assert.equal(
      found,
      undefined,
      `${file.relative} merges: ${found?.trim()}\n` +
        'Merging is the server\'s (ADR-0021). The engine applies what the server answers and ' +
        'surfaces the conflict; a merge rule here is a bug against that decision.',
    );
  }
});

// No Svelte, and nothing framework-shaped. The engine has to be exercisable headlessly, as the
// first-party counterpart to `hubctl sync-conformance` - and a package that imported a framework
// could not be (ADR-0033 §2).
test('no framework reaches this package', () => {
  const frameworks = /['"](?:svelte|svelte\/[\w-]+|react|vue|@sveltejs\/[\w-]+|solid-js)['"]/;
  for (const file of ALL) {
    const found = frameworks.exec(file.text);
    assert.equal(found, null, `${file.relative} imports ${found?.[0]} - the engine stays headless`);
  }
});

test('the manifest declares no framework either', () => {
  const manifest = JSON.parse(fs.readFileSync(path.join(packageRoot, 'package.json'), 'utf8'));
  const declared = Object.keys({
    ...manifest.dependencies,
    ...manifest.devDependencies,
    ...manifest.peerDependencies,
  });
  for (const name of declared) {
    assert.ok(
      !/^(?:svelte|react|vue|solid-js|@sveltejs\/)/.test(name),
      `package.json declares ${name}`,
    );
  }
  // The one workspace edge ADR-0033 §3 sanctions, and nothing else new.
  assert.deepEqual(Object.keys(manifest.dependencies ?? {}), ['@hubtask/api-client']);
});

// `fetch` in one file is what makes the bearer, the idempotency key and the deadline checkable at
// all. A second caller is a second place all three can be forgotten.
test('only the transport calls fetch', () => {
  for (const file of ALL) {
    if (file.relative.endsWith('FetchTransport.ts')) continue;
    // The fakes hand a `fetch` *in*; what is forbidden is reaching for the global.
    assert.doesNotMatch(
      file.text,
      /(?<![\w.])fetch\s*\(/,
      `${file.relative} calls fetch directly; every request goes through the Transport port`,
    );
  }
});
