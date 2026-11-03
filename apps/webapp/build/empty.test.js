// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One empty pattern (user-experience.md §5.5): an empty answer is `EmptyState` - for a product,
// `ProductEmpty` - and never a bare paragraph or a table's empty row holding a `*.empty` or
// `*_none` sentence. Read from the source, because the shape is what drifts: a new screen copies
// the paragraph from its neighbour.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const VIEWS = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src', 'views');

// A paragraph, or a table's `empty` snippet, whose content is a message code ending in `.empty`,
// `.none` or `_none` (a condition before the code included).
const BARE = /(?:<p\b[^>]*>|\{#snippet empty\(\)\})\s*\{[^}]*?\bt\('([a-z0-9_.]+(?:\.empty|\.none|_none))'/g;

// Lines inside a card, a band or a form that say a part of it is empty, where the screen around
// them is not: each is one sentence among others, not the answer of the screen's list.
const INLINE = new Set([
  'AdministrationOverviewView.svelte app.admin_overview.limits_none',
  'AdministrationOverviewView.svelte app.admin_overview.rules_none',
  'AdministrationOverviewView.svelte app.admin_overview.mail_none',
  'AdministrationOverviewView.svelte app.admin_overview.backup_none',
  'AdministrationOverviewView.svelte app.admin_overview.requests_none',
  'AdministrationOverviewView.svelte app.admin.check_none',
  'AdministrationOverviewView.svelte app.admin_overview.waiting_none',
  'TodayView.svelte app.today.mine_none',
  'TodayView.svelte app.today.step_none',
  'TodayView.svelte app.today.jumble_none',
  'InstanceSettingsView.svelte app.instance.settings_plan_none',
  'InstanceSettingsView.svelte app.instance.settings_history_none',
  'CmdbView.svelte app.cmdb_admin.aliases_none',
  'PortalRequestView.svelte app.portal.replies_none',
  'PrivacyView.svelte app.privacy.preview_none',
]);

test('no view draws an empty answer outside EmptyState', () => {
  const found = [];
  for (const name of fs.readdirSync(VIEWS).filter((file) => file.endsWith('.svelte'))) {
    const source = fs.readFileSync(path.join(VIEWS, name), 'utf8');
    for (const match of source.matchAll(BARE)) {
      const key = `${name} ${match[1]}`;
      if (!INLINE.has(key)) found.push(key);
    }
  }
  assert.deepEqual(found, [], 'draw these through EmptyState (a product: ProductEmpty), or name the line in INLINE with why');
});

test('every inline exception still exists, so the list cannot hide a new one by an old name', () => {
  for (const key of INLINE) {
    const [name, code] = key.split(' ');
    const source = fs.readFileSync(path.join(VIEWS, name), 'utf8');
    assert.ok(source.includes(`t('${code}'`), `${key} is no longer drawn; take it off the list`);
  }
});

// The product screens (§2.2, §5.1): each draws its empty answer through ProductEmpty.
const PRODUCT_VIEWS = [
  'ProductHomeView.svelte',
  'GoalsView.svelte',
  'ActualsView.svelte',
  'WorkloadView.svelte',
  'SprintView.svelte',
  'ReleaseView.svelte',
  'PortalView.svelte',
  'PublicRoadmapView.svelte',
];

test('every product screen draws its empty answer through ProductEmpty', () => {
  for (const name of PRODUCT_VIEWS) {
    const source = fs.readFileSync(path.join(VIEWS, name), 'utf8');
    assert.match(source, /<ProductEmpty\b/, `${name} draws no ProductEmpty`);
  }
});
