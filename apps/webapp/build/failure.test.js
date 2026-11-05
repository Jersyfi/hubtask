// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One failure pattern (user-experience.md §5.8): a refusal is `FailureNote`, a screen or list that
// could not be read is `FailureState`, and both fold the reference and the code behind *Details for
// support* with a copy control. Read from the source, because the shape is what drifts: a new screen
// copies the bare `ErrorState` or the hand-written reference line from its neighbour.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const SRC = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');

// The frame's own failures carry no problem from the server: a crash's local reference and a chunk
// that could not be loaded. FailureState is the one wrapper the rest goes through.
const MAY_DRAW_ERRORSTATE = new Set(['lib/frame/FailureState.svelte', 'lib/frame/ViewFailed.svelte', 'lib/frame/ChunkFailed.svelte']);
const MAY_SAY_REFERENCE = new Set(['lib/frame/FailureNote.svelte', 'lib/frame/FailureDetails.svelte']);

function sources(dir) {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return sources(full);
    return entry.name.endsWith('.svelte') ? [full] : [];
  });
}

test('no screen draws a failure outside FailureNote and FailureState', () => {
  const found = [];
  for (const file of sources(SRC)) {
    const name = path.relative(SRC, file).split(path.sep).join('/');
    const source = fs.readFileSync(file, 'utf8');
    if (/<ErrorState\b/.test(source) && !MAY_DRAW_ERRORSTATE.has(name)) found.push(`${name}: <ErrorState> - use FailureState`);
    if (source.includes("t('app.error_reference'") && !MAY_SAY_REFERENCE.has(name)) found.push(`${name}: the reference by hand - use FailureNote or FailureState`);
    // A danger banner that only says a kept sentence, or a problem's message, is a failure by hand.
    if (/<Banner tone="danger">\{[^}]*\}<\/Banner>/.test(source) || /<Banner tone="danger" title=\{[^}]*\.message[^}]*\}/.test(source)) {
      if (!MAY_SAY_REFERENCE.has(name)) found.push(`${name}: a failure in a bare danger Banner - use FailureNote`);
    }
  }
  assert.deepEqual(found, []);
});

test('every exception still exists, so the list cannot hide a new one by an old name', () => {
  for (const name of [...MAY_DRAW_ERRORSTATE, ...MAY_SAY_REFERENCE]) {
    assert.ok(fs.existsSync(path.join(SRC, name)), `${name} is gone; take it off the list`);
  }
});
