// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The other catalogues are loaded, not bundled (F5-07, milestone-F5.md decision 5).
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

  // Reachable from the entry, and only as a dynamic import: the source is what renders until
  // the reader's locale asks for it.
  const entry = fs.readFileSync(path.join(DIST, scripts[0]), 'utf8');
  assert.ok(entry.includes(`import(\`./${german}\`)`) || entry.includes(`import("./${german}")`), 'the entry does not load the German chunk on demand');
  assert.ok(!entry.includes(`from"./${german}"`) && !entry.includes(`from "./${german}"`), 'the entry imports the German chunk statically');
});
