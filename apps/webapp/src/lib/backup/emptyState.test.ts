// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The backup's empty state for its owner: the nightly backup is the one way, and a target of the
// owner's own the side way, which opens *Add a target* rather than only naming it. Read from the
// source, as the entry list's empty states are; that the press opens the dialog is not seen here.

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';

const source = readFileSync(new URL('../../views/BackupView.svelte', import.meta.url), 'utf8');

test('the empty backup offers a target of the owner\'s own as a side way that opens Add a target', () => {
  assert.match(
    source,
    /\{#snippet secondary\(\)\}\s*<Button tone="secondary" size="sm" icon="plus" onclick=\{\(\) => \(isAddingTarget = true\)\}>\{t\('app\.backup\.guided_own_target'\)\}<\/Button>/u,
  );
});
