// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The history names who each version is by, and the state before a scope's first apply as such.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { versionByCode } from './configversion.ts';

test('a version says whom it is by, and the state before the first apply is named', () => {
  const none = { added: 0, changed: 0, removed: 0 };
  assert.equal(versionByCode({ actor_kind: 'USER', ...none, added: 1 }), 'app.changes.by_person');
  assert.equal(versionByCode({ actor_kind: 'AI_AGENT', ...none, changed: 2 }), 'app.changes.by_agent');
  assert.equal(versionByCode({ actor_kind: 'DIRECT', ...none, removed: 1 }), 'app.changes.by_direct');
  assert.equal(versionByCode({ actor_kind: 'DIRECT', ...none }), 'app.changes.by_baseline');
});
