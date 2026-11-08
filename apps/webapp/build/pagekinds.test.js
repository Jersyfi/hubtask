// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Every routed view is one page kind (page-system PS-1, §11): it draws, itself or through the
// components it draws, the design system's page kinds of exactly one kind - `data-page-kind` on its
// root. A view of a migrated screen draws no `Table` and no raw table (a list is a `DataTable`,
// PS-4), and on a `ListPage` no field or form outside the `FilterBar` and a dialog (PS-3).
//
// Read from the source: the routed views are the ones the route table's three dispatchers and the
// lazy parts name.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

import { SRC, elements, read, reachable, relative, uses, within } from './markup.js';

// The kinds as the design system draws them, and the detail kind built as screens (§7.8).
const KINDS = {
  ListPage: 'list',
  RecordPage: 'record',
  SettingsLayout: 'settings',
  EditorPage: 'editor',
  OverviewPage: 'overview',
  LogPage: 'log',
  CreatePage: 'create',
  WorkPage: 'work',
};

/**
 * The routed views not yet migrated to a page kind. Later steps of the concept migrate them area
 * by area (page-system §13); this list may only shrink - a view that meets the rule must leave it,
 * which the last test holds - and it is empty when the concept lands.
 */
const NOT_YET_MIGRATED = [
  'ActualsView',
  'ApprovalsView', 'ArchiveView', 'AssetsView',
  'ConfirmAddressView', 'ConsentView', 'ContainerView', 'DashboardView',
  'GoalView', 'GoalsView',
  'HubDeskView', 'InstallationView', 'ItemView', 'JumbleView',
  'NotesView',
  'OidcCallbackView', 'PagesView', 'PlanView', 'PortalArticleView',
  'PortalRequestView', 'PortalView', 'PortfolioView',
  'ProductHomeView', 'PublicFormView', 'PublicRoadmapView',
  'RateView', 'RedeemView', 'ReleaseView', 'ResetView',
  'RoadmapView', 'SearchView',
  'SetupView', 'SignInView', 'SprintView',
  'TimeView', 'TodayView', 'TrashView', 'WhiteboardsView',
  'WorkloadView',
  'YourWorkspacesView',
];

const DISPATCHERS = ['App.svelte', 'views/AdministrationArea.svelte', 'views/InstanceArea.svelte', 'lib/frame/parts.ts'];

/** The views a route draws: every `views/…View.svelte` a dispatcher names. */
function routedViews() {
  const names = new Set();
  for (const dispatcher of DISPATCHERS) {
    const source = fs.readFileSync(path.join(SRC, dispatcher), 'utf8');
    for (const match of source.matchAll(/\b(\w+View)\.svelte/g)) {
      if (fs.existsSync(path.join(SRC, 'views', `${match[1]}.svelte`))) names.add(match[1]);
    }
  }
  return [...names].sort();
}

const fileOf = (view) => path.join(SRC, 'views', `${view}.svelte`);

/** The kinds a view draws, through everything it draws. */
function kindsOf(view) {
  const kinds = new Set();
  for (const file of reachable(fileOf(view))) {
    const { markup } = read(file);
    for (const [component, kind] of Object.entries(KINDS)) {
      if (new RegExp(`<${component}(?=[\\s/>])`).test(markup)) kinds.add(kind);
    }
    for (const match of markup.matchAll(/data-page-kind="(work|detail)"/g)) kinds.add(match[1]);
  }
  return kinds;
}

/** A raw table or the bare `Table` anywhere a view draws. */
function tablesOf(view) {
  const found = [];
  for (const file of reachable(fileOf(view))) {
    const { markup } = read(file);
    if (/<Table(?=[\s/>])|<table(?=[\s>])/.test(markup)) found.push(relative(file));
  }
  return found;
}

const FIELDS = /<(Input|Select|Textarea|form)(?=[\s/>])/g;

/** Where the dialogs and the filter bar of a file are: a field inside one is in its place. */
function sheltered(markup) {
  return [...elements(markup, 'Dialog'), ...elements(markup, 'FilterBar')];
}

/** Fields a view draws outside a dialog and the filter bar, following the components it draws. */
function openFields(file, isSheltered = false, seen = new Map()) {
  const key = `${file}|${isSheltered}`;
  if (seen.has(key)) return [];
  seen.set(key, true);
  const { markup } = read(file);
  const ranges = sheltered(markup);
  const found = [];
  if (!isSheltered) {
    for (const match of markup.matchAll(FIELDS)) {
      if (!within(ranges, match.index)) found.push(`${relative(file)}: ${match[1]}`);
    }
  }
  for (const use of uses(file)) {
    if (!fs.existsSync(use.file)) continue;
    found.push(...openFields(use.file, isSheltered || within(ranges, use.at), seen));
  }
  return found;
}

const migrated = () => routedViews().filter((view) => !NOT_YET_MIGRATED.includes(view));

test('every migrated routed view draws exactly one page kind', () => {
  const wrong = migrated()
    .map((view) => ({ view, kinds: [...kindsOf(view)] }))
    .filter(({ kinds }) => kinds.length !== 1)
    .map(({ view, kinds }) => `${view}: ${kinds.length === 0 ? 'no page kind' : kinds.join(', ')}`);
  assert.deepEqual(wrong, []);
});

test('a migrated view draws no bare Table and no raw table - a list is a DataTable', () => {
  const wrong = migrated().flatMap((view) => tablesOf(view).map((file) => `${view} → ${file}`));
  assert.deepEqual([...new Set(wrong)], []);
});

test('a migrated list page opens no field or form outside its filter bar and a dialog', () => {
  const wrong = migrated()
    .filter((view) => kindsOf(view).has('list') || kindsOf(view).has('log'))
    .flatMap((view) => openFields(fileOf(view)).map((field) => `${view} → ${field}`));
  assert.deepEqual([...new Set(wrong)], []);
});

test('the list of views not yet migrated only shrinks', () => {
  const routed = routedViews();
  const unknown = NOT_YET_MIGRATED.filter((view) => !routed.includes(view));
  assert.deepEqual(unknown, [], 'a view that is no longer routed leaves the list');
  const done = NOT_YET_MIGRATED.filter((view) => kindsOf(view).size === 1 && tablesOf(view).length === 0);
  assert.deepEqual(done, [], 'a view that draws one page kind and no bare table leaves the list');
});
