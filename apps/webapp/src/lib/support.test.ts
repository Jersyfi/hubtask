// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { forgetReferences, recentReferences, rememberReference, supportDetails } from './support.ts';

test('the last few references are kept, newest first and each once', () => {
  forgetReferences();
  for (const id of ['a1', 'b2', 'a1', 'c3', 'd4', 'e5', 'f6']) rememberReference(id);
  assert.deepEqual(recentReferences(), ['f6', 'e5', 'd4', 'c3', 'a1']);
  rememberReference('');
  assert.deepEqual(recentReferences(), ['f6', 'e5', 'd4', 'c3', 'a1']);
});

test('the details for support carry no content: version, workspace, route and references only', () => {
  const text = supportDetails({
    version: '1.4.0',
    workspace: 'acme',
    route: '/items/01a0e2e0-0000-7000-8000-000000000001?q=salary%20review',
    references: ['f6', 'e5'],
    labels: { version: 'Version', workspace: 'Workspace', route: 'Page', references: 'References' },
  });
  assert.equal(text, 'Version: 1.4.0\nWorkspace: acme\nPage: /items/01a0e2e0-0000-7000-8000-000000000001\nReferences: f6, e5');
  // A query or a fragment can hold what somebody searched for (rule 10's spirit): never copied.
  assert.doesNotMatch(text, /salary/);
});

test('what is not known is said as a dash, never left out', () => {
  const text = supportDetails({ version: undefined, workspace: undefined, route: '/today#x', references: [], labels: { version: 'V', workspace: 'W', route: 'P', references: 'R' } });
  assert.equal(text, 'V: —\nW: —\nP: /today\nR: —');
});
