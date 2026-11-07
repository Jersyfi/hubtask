// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A page asks only what its reader may read: a read the server would refuse - by the reader's role
// or because its module is off - is a failure in the console, not a quiet absence. Each walk
// refuses those reads as the server does and holds the page to a clean console.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { COLLECTION, PERMISSIONS } from './fixture.mjs';
import { productWalk } from './productwalk.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const refusing = (paths) => (method, path) => (paths.includes(path) ? { status: 403, json: { type: 'about:blank', title: 'Forbidden', status: 403, code: 'access.not_permitted' } } : undefined);

const walks = [
  {
    name: 'a member on the desk\'s home asks no template catalogue',
    modules: ['SERVICE_DESK', 'WORKFLOWS'], manifest: { workspace_permissions: PERMISSIONS.MEMBER, private_hub_offered: true },
    refused: ['/config/templates'], paths: ['/desk'],
  },
  {
    name: 'a member on the agents asks none of the administration\'s reads',
    modules: ['AGENTS'], manifest: { workspace_permissions: PERMISSIONS.MEMBER },
    refused: ['/agent-models', '/agents:halt', '/agents:policy', '/agents:metering', '/ai-source'], paths: ['/agents', '/profile/agents'],
  },
  {
    name: 'an owner of a workspace with every module off asks no module\'s read',
    modules: [], manifest: {},
    refused: ['/approvals', '/pages:personal', '/pages:shared', '/pages:public-links', '/portfolios:public-links', '/public-forms:policy', '/entry-types', '/workflows'],
    paths: ['/approvals', '/notes', '/administration/workspace', '/', `/collections/${COLLECTION.id}`],
  },
  {
    // The fixture's workspace configured no provider: its source says so, and the read that would
    // answer 404 is not asked.
    name: 'an owner on the AI settings asks no provider the source says is not there',
    modules: [], manifest: {}, refused: [], paths: ['/administration/ai'],
  },
];

for (const walkOf of walks) {
  test(`chromium: ${walkOf.name}`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const walk = await productWalk(browser, { modules: walkOf.modules, manifest: walkOf.manifest, answer: refusing(walkOf.refused) });
    t.after(walk.close);
    const { page } = walk;
    for (const path of walkOf.paths) {
      await page.goto(`${served.origin}${path}`);
      await page.waitForLoadState('networkidle');
      assert.deepEqual(walk.errors(), [], `on ${path}`);
    }
  });
}
