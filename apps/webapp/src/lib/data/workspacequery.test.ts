// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { TENANT_PAGE, workspaceQuery } from './workspacequery.ts';

test('the workspaces screen asks one page, narrowed by what was given and nothing else', () => {
  assert.deepEqual(workspaceQuery({}), { size: TENANT_PAGE });
  assert.ok(TENANT_PAGE <= 200, 'a page past the contract\'s maximum is refused');
  assert.deepEqual(workspaceQuery({ slug: '  Acme ', status: 'SUSPENDED', plan: 'team' }), {
    slug: 'acme', status: 'SUSPENDED', plan: 'team', size: TENANT_PAGE,
  });
  assert.deepEqual(workspaceQuery({ plan: '-' }), { without_plan: true, size: TENANT_PAGE });
  assert.deepEqual(workspaceQuery({ slug: ' ', plan: '' }), { size: TENANT_PAGE });
  assert.deepEqual(workspaceQuery({ slug: 'acme' }, 1), { slug: 'acme', size: 1 });
});
