// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A page's formula, diagram and highlighted code (fourth answers, 2) are drawn under the product's
// content security policy: the formula as MathML, the diagram as a picture, the code in colours,
// each library loaded only once the page holds such a block.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, HUB, signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
// The interface's policy as presentation/webui sends it: no inline style, no eval.
const POLICY = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; font-src 'self'; connect-src 'self'; " +
  "manifest-src 'self'; worker-src 'self' blob:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'";
const served = await serve(DIST, { headers: { 'content-security-policy': POLICY } });
test.after(() => served.close());

const PAGE_ID = '01a0e2e0-0000-7000-8000-000000000951';
const PAGE = {
  id: PAGE_ID, hub_id: HUB.id, parent_id: null, title: 'Formulas', icon: null, position: 'a1', template: false, content_language: null, owner_id: null,
  verified_until: null, verified_at: null, verified_by: null, due_for_review: false, archived: false, restricted: false, daily_for: null,
  trashed_at: null, created_by: ACCOUNT.id, created_at: '2026-10-01T09:00:00Z', updated_by: ACCOUNT.id, updated_at: '2026-10-02T09:00:00Z', version: 1,
};
const BODY = ['$$', 'a^2 + b^2 = c^2', '$$', '', '```mermaid', 'graph TD', '  A[Request] --> B[Done]', '```', '', '```go', 'func main() {}', '```', ''].join('\n');
const empty = { data: [], page: { next_cursor: null, has_more: false } };

test('a formula, a diagram and coloured code are drawn under the policy', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, context } = await signedIn(browser, 1280, 900);
    const refused = [];
    page.on('console', (message) => { if (/Content Security Policy|Refused to/u.test(message.text())) refused.push(message.text()); });
    await context.unroute('**/api/v1/**');
    await context.route('**/api/v1/**', async (route) => {
      const path = new URL(route.request().url()).pathname.replace('/api/v1', '');
      if (path === `/hubs/${HUB.id}/pages`) return route.fulfill({ json: { ...empty, data: [PAGE] } });
      if (path === `/hubs/${HUB.id}/page-labels` || path === `/hubs/${HUB.id}/trashed-pages`) return route.fulfill({ json: empty });
      if (path === `/hubs/${HUB.id}/page-drafts`) return route.fulfill({ json: { hub_id: HUB.id, drafts: false } });
      if (path === `/pages/${PAGE_ID}`) {
        return route.fulfill({ json: { ...PAGE, body: BODY, ancestors: [], writable: false, shared: false, drafts: false, unpublished: false, draft: null } });
      }
      if (/^\/pages\/[^/]+\/(comments|versions|backlinks)$/.test(path)) return route.fulfill({ json: empty });
      if (/^\/pages\/[^/]+\/language-variants$/.test(path)) return route.fulfill({ json: { page_id: PAGE_ID, source_page_id: null, language: null, variants: [] } });
      if (/^\/pages\/[^/]+\/labels$/.test(path)) return route.fulfill({ json: { page_id: PAGE_ID, labels: [] } });
      return stub(route);
    });
    await page.goto(`${served.origin}/pages/${PAGE_ID}`);
    await page.locator('.page-math__formula math').waitFor({ timeout: 20_000 });
    await page.locator('.hljs-keyword').first().waitFor({ timeout: 20_000 });
    const picture = page.locator('.page-diagram__picture');
    await picture.waitFor({ timeout: 20_000 });
    assert.match(await picture.getAttribute('src') ?? '', /^data:image\/svg\+xml/u);
    assert.ok(await picture.evaluate((image) => image.complete && image.naturalWidth > 0), 'the diagram picture loads');
    assert.equal(await page.locator('.page-diagram--error, .page-math__formula--error').count(), 0);
    assert.deepEqual(failures, []);
    // Mermaid measures its labels in the page and sets style attributes there; the policy refuses
    // them, which is what keeps the page unstyled by it. Nothing else may be refused - no script.
    assert.deepEqual(refused.filter((text) => !/^Applying inline style violates/u.test(text)), []);
    await context.close();
  } finally {
    await browser.close();
  }
});
