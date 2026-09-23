// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The other half of F5-05: with AI switched off in the manifest, the product never mentioned it.
//
// `design-system.md` §4 asks `AISuggestion` to disappear "without residue" when AI is off, and
// every screen this milestone gave a proposal to rendered it behind the same fact -
// `features.ai_suggestions` in `/meta/capabilities`. This proves it for the whole client rather
// than per component: every view is rendered on the server with a fake manifest, once with AI off
// and once with it on, and the HTML is searched for `data-ai` - the mark every AI root carries,
// which is what the `ai.*` tokens style and what asks. Off finds nothing on any route; on finds it
// where a proposal belongs, which is what keeps the first assertion from passing by accident.
//
// Rendered rather than read: a grep for `hasAi` would trust that every consumer was guarded, and
// this asks the components what they draw.

import { register } from 'node:module';
import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

register('./svelte-loader.js', import.meta.url);

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

const TENANT = '0192f000-0000-7000-8000-0000000000a1';
const COLLECTION = '0192f000-0000-7000-8000-00000000c001';
const ITEM = '0192f000-0000-7000-8000-00000000e001';
const ENTRY = '0192f000-0000-7000-8000-00000000f001';

/** The manifest as the residue test needs it: the AI flags, and enough of the rest to render. */
const manifestWith = (ai) => ({
  version: '0.0.0-test',
  item_types: [
    { type: 'TASK', capabilities: ['assignee', 'due_date', 'labels', 'bucket'], child_types: ['WORK_PACKAGE'] },
    { type: 'WORK_PACKAGE', capabilities: ['due_date'], child_types: ['ACTIVITY'] },
    { type: 'ACTIVITY', capabilities: [], child_types: [] },
  ],
  view_layouts: ['LIST_COLLAPSED', 'LIST_EXPANDED', 'KANBAN', 'TIMELINE'],
  query_fields: [],
  supported_locales: [{ locale: 'en', direction: 'ltr' }],
  text_languages: ['en'],
  roles: [],
  limits: {},
  features: { ai_suggestions: ai, semantic_search: ai, mail: false, storage: true, web_ui: true },
  token_scopes: [],
});

const item = {
  id: ITEM,
  type: 'TASK',
  collection_id: COLLECTION,
  parent_id: null,
  path: `/${ITEM}/`,
  depth: 0,
  title: 'A task',
  notes: null,
  completion: { is_completed: false },
  bucket_id: null,
  order_key: 'a',
  due_at: null,
  due_date_only: false,
  due_time_zone: null,
  label_ids: [],
  member_ids: [],
  assignee_id: null,
  custom_fields: {},
  archived_at: null,
  deleted_at: null,
  created_by: '0192f000-0000-7000-8000-00000000000d',
  created_at: '2026-09-16T10:00:00Z',
  updated_at: '2026-09-16T10:00:00Z',
  version: 1,
};

const collection = {
  id: COLLECTION,
  type: 'COLLECTION',
  parent_id: '0192f000-0000-7000-8000-00000000b001',
  name: 'A collection',
  description: null,
  order_key: 'a',
  archived_at: null,
  created_at: '2026-09-16T10:00:00Z',
  version: 1,
};

const entry = {
  id: ENTRY,
  channel: 'QUICK_CAPTURE',
  sender: null,
  raw_subject: 'An arrival',
  raw_body: 'Something to decide about.',
  attachments: [],
  status: 'NEW',
  target_item_id: null,
  received_at: '2026-09-16T10:00:00Z',
  settled_at: null,
};

/** What the fake server answers, by path. Everything else is `404`, which every screen tolerates. */
function answerFor(url, ai) {
  const { pathname } = new URL(url, 'http://localhost');
  const table = {
    '/api/v1/meta/capabilities': manifestWith(ai),
    '/api/v1/accounts/me': { id: '0192f000-0000-7000-8000-00000000000d', tenant_id: TENANT, kind: 'USER', display_name: 'Tester', status: 'ACTIVE', locale: 'en', time_zone: 'UTC', version: 1 },
    [`/api/v1/items/${ITEM}`]: item,
    [`/api/v1/containers/${COLLECTION}`]: collection,
    '/api/v1/jumble/entries': { data: [entry], next_cursor: null },
    '/api/v1/suggestions': { items: [], has_more: false },
    '/api/v1/items:query': { data: [], page: { has_more: false, next_cursor: null } },
    '/api/v1/containers': { data: [], page: { has_more: false, next_cursor: null } },
  };
  return table[pathname];
}

/**
 * A `fetch` that answers the table above. The application's one transport is `FetchTransport`
 * over the global `fetch`, bound once when the engine is constructed - so the function is
 * installed once, and which manifest it answers is a switch it reads on every call.
 */
let aiOn = false;
function installFetch(ai) {
  aiOn = ai;
}
globalThis.fetch = async (input) => {
  const url = typeof input === 'string' ? input : input.url;
  const body = answerFor(url, aiOn);
  if (body === undefined) {
    return new Response(JSON.stringify({ code: 'not_found', title: 'not_found', status: 404 }), {
      status: 404,
      headers: { 'content-type': 'application/problem+json' },
    });
  }
  return new Response(JSON.stringify(body), { status: 200, headers: { 'content-type': 'application/json', etag: 'W/"1"' } });
};

// The browser globals the modules touch at import or at render. `sessionStorage` holds the
// bearer the session reads; `window`/`document` are what the router and the theme touch and
// are stubbed to the least that lets a render proceed.
globalThis.sessionStorage = new (class {
  #held = new Map([['hubtask.bearer', 'hbt_pat_test'], ['hubtask.refresh', 'none']]);
  getItem(key) { return this.#held.get(key) ?? null; }
  setItem(key, value) { this.#held.set(key, String(value)); }
  removeItem(key) { this.#held.delete(key); }
})();
globalThis.localStorage = globalThis.sessionStorage;
globalThis.window = globalThis;
globalThis.location = new URL('http://localhost/');
globalThis.history = { pushState() {}, replaceState() {}, state: null };
Object.defineProperty(globalThis, 'navigator', { value: { languages: ['en'], language: 'en', onLine: true }, configurable: true });
globalThis.matchMedia = () => ({ matches: false, addEventListener() {}, removeEventListener() {} });
globalThis.document = {
  documentElement: { setAttribute() {}, removeAttribute() {}, getAttribute() { return null; } },
  addEventListener() {},
  removeEventListener() {},
  createElement() { return { setAttribute() {}, style: {} }; },
  body: { append() {} },
};
globalThis.addEventListener = () => {};
globalThis.removeEventListener = () => {};
globalThis.crypto ??= { randomUUID: () => '0192f000-0000-7000-8000-000000000000' };

/** The stores that read once at boot, started and awaited, so the views render against an answer. */
async function boot() {
  const { manifest } = await import('../src/lib/data/capabilities.svelte.ts');
  const { engine } = await import('../src/lib/data/engine.ts');
  engine.reset?.();
  const { jumble } = await import('../src/lib/data/jumble.svelte.ts');
  const { containers } = await import('../src/lib/data/containers.svelte.ts');
  const { actor } = await import('../src/lib/data/account.svelte.ts');
  const stops = [manifest.start(), actor.start()];
  await engine.refresh({ path: '/meta/capabilities' });
  await engine.refresh({ path: '/accounts/me' });
  // The entry as the view reads it - with the labels expanded (issue 875) - primed under that key.
  const { itemPath } = await import('../src/lib/data/item.svelte.ts');
  await engine.refresh({ path: itemPath(ITEM) });
  await engine.refresh({ path: `/containers/${COLLECTION}` });
  // The inbox reads through its own store rather than a `resource()`, and a store fills from its
  // subscription - which an effect starts in the browser and this starts here.
  stops.push(jumble.open('NEW'));
  await engine.refresh({ path: '/jumble/entries?status=NEW' });
  // The collection screen reads its container through the containers store the same way.
  stops.push(containers.openSingle(COLLECTION));
  await engine.refresh({ path: `/containers/${COLLECTION}` });
  return () => stops.forEach((stop) => stop());
}

/** Every route of the application, with what each screen takes to render. */
async function views() {
  const load = async (name) => (await import(`../src/views/${name}.svelte`)).default;
  const noop = () => {};
  return [
    ['home', await load('HomeView'), {}],
    ['installation', await load('InstallationView'), {}],
    ['profile', await load('ProfileView'), {}],
    ['tokens', await load('MyTokensView'), {}],
    ['appearance', await load('AppearanceView'), {}],
    ['notifications', await load('NotificationsView'), {}],
    ['security', await load('SecurityView'), {}],
    ['sessions', await load('SessionsView'), {}],
    ['devices', await load('DevicesView'), {}],
    ['grants', await load('GrantsView'), {}],
    ['workspace-settings', await load('WorkspaceSettingsView'), {}],
    ['people', await load('PeopleView'), {}],
    ['groups', await load('GroupsView'), {}],
    ['permissions', await load('PermissionsView'), {}],
    ['service-accounts', await load('ServiceAccountsView'), {}],
    ['apps', await load('AppsView'), {}],
    ['rules', await load('RulesView'), {}],
    ['rule-new', await load('RuleEditorView'), { id: 'new', onnavigate: noop }],
    ['rule', await load('RuleEditorView'), { id: ITEM, onnavigate: noop }],
    ['runs', await load('RunsView'), {}],
    ['webhooks', await load('WebhooksView'), {}],
    ['quotas', await load('QuotasView'), {}],
    ['backup', await load('BackupView'), {}],
    ['retention', await load('RetentionView'), {}],
    ['restore', await load('RestoreView'), {}],
    ['audit', await load('AuditView'), {}],
    ['privacy', await load('PrivacyView'), {}],
    ['identity-provider', await load('IdentityProviderView'), {}],
    ['ai', await load('AiSettingsView'), {}],
    ['search', await load('SearchView'), {}],
    ['jumble', await load('JumbleView'), { onnavigate: noop }],
    ['trash', await load('TrashView'), {}],
    ['archive', await load('ArchiveView'), { onnavigate: noop }],
    ['item', await load('ItemView'), { id: ITEM }],
    ['collection', await load('ContainerView'), { id: COLLECTION, onnavigate: noop }],
    ['sign-in', await load('SignInView'), {}],
    ['redeem', await load('RedeemView'), { onnavigate: noop }],
    ['consent', await load('ConsentView'), {}],
  ];
}

test('the route table and this test name the same screens', async () => {
  const { ROUTES } = await import('../src/lib/routes.ts');
  // `sign-in` is a screen without a route: it stands in for every route while nobody is signed
  // in, and is rendered here for that reason.
  const named = (await views()).map(([name]) => name).filter((name) => name !== 'sign-in').sort();
  const routed = ROUTES.map((route) => route.name)
    // `hub` is `ContainerView` as `collection` is; `oidc-callback` finishes an exchange and draws
    // nothing of the product; `administration` is a section's front door and draws nothing either -
    // it replaces the address with the section's first screen (ADR-0065 decision 1).
    .filter((name) => name !== 'hub' && name !== 'oidc-callback' && name !== 'administration')
    .sort();
  assert.deepEqual(named, routed);
});

test('with AI off, no route renders anything the AI tokens style or anything that asks', async () => {
  installFetch(false);
  const { render } = await import('svelte/server');
  const stop = await boot();
  try {
    for (const [name, View, props] of await views()) {
      const { body } = render(View, { props });
      assert.ok(!body.includes('data-ai'), `${name} renders an AI element with AI off`);
    }
  } finally {
    stop();
  }
});

test('every screen renders exactly one h1 and no main of its own; the frame renders the one main', async () => {
  // 2.4.1 and 1.3.1, as the F5-11 walk read them: one landmark to skip to, one heading that names
  // the page. A screen that rendered its own `<main>` would give a reader two, and a screen with
  // two `<h1>` - or none while it loads - would name itself twice or not at all.
  installFetch(false);
  const { render } = await import('svelte/server');
  const stop = await boot();
  try {
    for (const [name, View, props] of await views()) {
      const { body } = render(View, { props });
      const headings = body.match(/<h1[\s>]/g)?.length ?? 0;
      assert.equal(headings, 1, `${name} renders ${headings} <h1> elements`);
      assert.ok(!/<main[\s>]/.test(body), `${name} renders a <main> of its own`);
    }
  } finally {
    stop();
  }
  // The frame is not rendered here - it takes a session and a router - so the one `<main>` is
  // asserted where it is written, with the skip link that lands on it.
  const frame = fs.readFileSync(path.join(ROOT, 'src', 'lib', 'frame', 'AppFrame.svelte'), 'utf8');
  assert.equal(frame.match(/<main[\s>]/g)?.length, 1, 'the frame renders exactly one <main>');
  assert.ok(/<main id="main" tabindex="-1"/.test(frame), 'the main landmark is where the skip link lands');
  assert.ok(/href="#main"/.test(frame), 'the frame has a skip link to the main landmark');
});

test('with AI on, the screens that carry a proposal render it - so the first test cannot pass by accident', async () => {
  installFetch(true);
  const { render } = await import('svelte/server');
  const { engine } = await import('../src/lib/data/engine.ts');
  engine.reset();
  const stop = await boot();
  try {
    const carrying = new Set(['item', 'jumble', 'collection']);
    for (const [name, View, props] of await views()) {
      const { body } = render(View, { props });
      if (carrying.has(name)) assert.ok(body.includes('data-ai'), `${name} renders no AI element with AI on`);
    }
  } finally {
    stop();
  }
});

test('the AI tokens are consumed by AISuggestion and nothing else', () => {
  // The mark above is what the render finds; this is what makes the mark complete. A component
  // that used `--ai-*` without carrying `data-ai` would be residue the render could not see.
  const consumers = [];
  const walk = (dir) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(full);
      // design-system-lint-ignore: a prefix the test searches for, not a property it reads
      else if (/\.(svelte|ts|css)$/.test(entry.name) && fs.readFileSync(full, 'utf8').includes('var(--ai-')) consumers.push(path.relative(ROOT, full));
    }
  };
  walk(path.join(ROOT, 'src'));
  walk(path.join(ROOT, '..', '..', 'packages', 'design-system', 'src'));
  assert.deepEqual(consumers, ['../../packages/design-system/src/AISuggestion.svelte']);
});
