// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The other catalogues are loaded, not bundled (`i18n-l10n.md` §3).
//
// `import.meta.glob` over `locales/*.json` gives one chunk per file and none of them in the
// initial bundle - which is a promise about what Vite emits, so it is checked against what Vite
// emitted. The entry `index.html` references is read, and the German chunk has to be reachable
// from it only through a dynamic `import()`, never as a static import or a script tag. Where no
// build exists - a test run before one - this says so and skips rather than passing on nothing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const DIST = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const built = fs.existsSync(path.join(DIST, 'index.html'));

test('the German catalogue is a chunk of its own, outside the initial bundle', { skip: !built && 'no build in dist/' }, () => {
  const html = fs.readFileSync(path.join(DIST, 'index.html'), 'utf8');
  const scripts = [...html.matchAll(/<script[^>]*\ssrc="\/?([^"]+)"/g)].map((m) => m[1]);
  const preloads = [...html.matchAll(/<link[^>]*rel="modulepreload"[^>]*href="\/?([^"]+)"/g)].map((m) => m[1]);
  const initial = [...scripts, ...preloads];
  assert.ok(initial.length > 0, 'index.html references no script');
  assert.ok(!initial.some((file) => /\/de-[^/]*\.js$/.test(file)), `the German chunk is loaded up front: ${initial.join(', ')}`);

  const assets = fs.readdirSync(path.join(DIST, 'assets'));
  const german = assets.find((file) => /^de-[^.]*\.js$/.test(file));
  assert.ok(german, 'no chunk for locales/de.json was emitted; the glob found nothing');

  // Reachable from the initial bundle, and only as a dynamic import: the source is what renders
  // until the reader's locale asks for it. The initial bundle is the entry and every chunk it
  // imports statically - the bundler may put the catalogue's loader in a chunk the entry shares
  // with a lazy screen, which is still loaded up front.
  const initialChunks = new Set();
  const pending = [scripts[0]];
  while (pending.length > 0) {
    const file = pending.pop();
    if (initialChunks.has(file)) continue;
    initialChunks.add(file);
    const source = fs.readFileSync(path.join(DIST, file), 'utf8');
    for (const m of source.matchAll(/(?:from|import)\s*"\.\/([^"]+\.js)"/g)) pending.push(path.posix.join(path.posix.dirname(file), m[1]));
  }
  const sources = [...initialChunks].map((file) => fs.readFileSync(path.join(DIST, file), 'utf8'));
  assert.ok(!initialChunks.has(`assets/${german}`), `the German chunk is in the initial bundle: ${[...initialChunks].join(', ')}`);
  assert.ok(sources.some((source) => source.includes(`import(\`./${german}\`)`) || source.includes(`import("./${german}")`)), 'the initial bundle does not load the German chunk on demand');
  assert.ok(sources.every((source) => !source.includes(`from"./${german}"`) && !source.includes(`from "./${german}"`)), 'the initial bundle imports the German chunk statically');
});
