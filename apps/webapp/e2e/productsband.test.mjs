// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The products band and a product's home (user-experience.md §2.1, §2.2, §2.6): one row per
// product switched on, in one order, under *Products*; none where none is on; the home lists the
// hubs where the product is on and says what to do where there are none; on a phone *More* opens
// the drawer whose first band is the products. Walked at desk and phone width, light and dark.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { MANIFEST, PERMISSIONS, ROLES, fallback } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const ACCOUNT = { id: '01a0e2e0-0000-7000-8000-000000000001', kind: 'USER', display_name: 'Band Walker', email: 'band@example.invalid', status: 'ACTIVE', locale: 'en', onboarding_completed_at: '2026-09-01T00:00:00Z' };
const HUB = { id: '01a0e2e0-0000-7000-8000-000000000002', type: 'HUB', parent_id: null, name: 'Product development and delivery for the whole company', order_key: 'a0', version: 1 };
const PAGE = { data: [], items: [], page: { next_cursor: null, has_more: false } };
const board = (n, title, at) => ({ id: `01a0e2e0-0000-7000-8000-0000000001${n}`, hub_id: HUB.id, title, element_count: 3, trashed_at: null, created_by: ACCOUNT.id, created_at: at, updated_by: ACCOUNT.id, updated_at: at, version: 1 });
const agent = (n, name, status, owner) => ({ id: `01a0e2e0-0000-7000-8000-0000000002${n}`, account_id: null, name, purpose: 'Sorts new requests into the right queue', kind: 'TICKET_TRIAGE', owner_id: owner, status,
  risk_class: 'MINIMAL', scopes: [], model_id: null, policy: { reversible: 'APPROVAL', irreversible: 'APPROVAL' }, budget: { tokens_per_day: 200000, actions_per_hour: 120, concurrent_runs: 2 },
  min_accuracy: 0, needs_approval: false, approved_by: null, approved_at: null, suspended_by: null, suspended_at: null, created_at: '2026-11-01T08:00:00Z', version: 1 });
const AGENTS = [agent('01', 'Old sorter', 'RETIRED', ACCOUNT.id), agent('02', 'Request sorter', 'ACTIVE', ACCOUNT.id), agent('03', 'Duplicate finder', 'SUSPENDED', '01a0e2e0-0000-7000-8000-0000000009ff')];
const HELD = { id: '01a0e2e0-0000-7000-8000-000000000211', agent_id: AGENTS[1].id, use_case: 'UpdateItem', class: 'REVERSIBLE', input: {}, status: 'PENDING', decided_by: null, decided_at: null, code: '', created_at: '2026-11-03T08:00:00Z' };
const job = (n, status, at) => ({ id: `01a0e2e0-0000-7000-8000-00000000022${n}`, agent_id: AGENTS[1].id, target_type: 'ENTRY', target_id: '01a0e2e0-0000-7000-8000-000000000299', status, code: '', proposals: 1, tokens: 120, started_by: null, started_at: at, ended_at: null });
const JOBS = [job(1, 'FAILED', '2026-11-02T08:00:00Z'), job(2, 'DONE', '2026-11-03T08:00:00Z')];
const pageOf = (n, title, at, more = {}) => ({ id: `01a0e2e0-0000-7000-8000-0000000003${n}`, hub_id: HUB.id, parent_id: null, title, icon: null, position: n, template: false, content_language: null, owner_id: null, verified_until: null, verified_at: null, verified_by: null, due_for_review: false, archived: false, restricted: false, daily_for: null, trashed_at: null, created_by: ACCOUNT.id, created_at: at, updated_by: ACCOUNT.id, updated_at: at, version: 1, ...more });
const PAGES = [
  pageOf('01', 'Onboarding guide', '2026-10-02T09:00:00Z'),
  pageOf('02', 'How we release, step by step, for every team that ships to customers', '2026-10-31T09:00:00Z'),
  pageOf('03', 'Meeting notes template', '2026-11-01T09:00:00Z', { template: true }),
  pageOf('04', 'Support rota', '2026-09-01T09:00:00Z', { owner_id: ACCOUNT.id, due_for_review: true }),
];
const HEALTH = { total: 42, completeness: 0.9, correctness: 0.8, stale: 2, orphans: 1, duplicates: 0, without_owner: 3, review_overdue: 4, classes: [{ class: 'laptop', total: 12, completeness: 1, stale: 0, orphans: 0, duplicates: 0 }, { class: 'server', total: 30, completeness: 0.9, stale: 2, orphans: 1, duplicates: 0 }], problems: [{ ci_id: '01a0e2e0-0000-7000-8000-000000000401', class: 'server', kinds: ['STALE'] }, { ci_id: '01a0e2e0-0000-7000-8000-000000000402', class: 'server', kinds: ['NO_OWNER'] }], stale_relations: 0, services: [], sources: [] };
// The handler's own deadlines kept: four kinds, so the card draws three and "+ 1 more".
const KEPT = [
  { kind: 'SLA', goal: 'FIRST_RESPONSE', group_id: null, ended: 2, kept: 1 },
  { kind: 'SLA', goal: 'RESOLUTION', group_id: null, ended: 4, kept: 4 },
  { kind: 'SLA', goal: 'NEXT_RESPONSE', group_id: null, ended: 5, kept: 4 },
  { kind: 'OLA', goal: 'RESOLUTION', group_id: '01a0e2e0-0000-7000-8000-000000000901', ended: 3, kept: 1 },
];
const DESK = { id: '01a0e2e0-0000-7000-8000-000000000501', type: 'COLLECTION', parent_id: HUB.id, name: 'IT help desk for every office and every remote worker', handle: 'it', key_prefix: 'INC', order_key: 'a0', version: 1, structure: { settings: { entry_types: { TASK: ['incident'] } } } };
const SCRUM = { id: '01a0e2e0-0000-7000-8000-000000000502', type: 'COLLECTION', parent_id: HUB.id, name: 'Sprint work', order_key: 'a1', version: 1, structure: { settings: { entry_types: { TASK: ['story'] } } } };
const TYPES = [
  { key: 'incident', level: 'TASK', name: 'Incident', system: false, position: 1, archived: false, form: { channels: [{ channel: 'PORTAL' }] } },
  { key: 'story', level: 'TASK', name: 'Story', system: false, position: 2, archived: false },
];
const ticket = (n, title) => ({ id: `01a0e2e0-0000-7000-8000-0000000006${n}`, type: 'TASK', entry_type: 'incident', collection_id: DESK.id, title, number: Number(n), created_by: ACCOUNT.id, custom_fields: { priority: `P${Number(n) + 1}` }, completion: { mode: 'BINARY', is_completed: false }, version: 1 });
/** Each ticket's goals: the first running with an hour left, the second breached, the third paused. */
const goalOf = (standing, due, breached) => ({ goal: 'RESOLUTION', kind: 'SLA', policy_key: 'standard', assignee_group: null, standing, target_seconds: 7200, armed_at: new Date(Date.now() - 3_600_000).toISOString(), due_at: due, at_risk_at: null, met_at: null, breached_at: breached });
// Built when asked, not when the file loads: a slow run reaches the desk minutes later, and the
// words the page says ("18 min ago") are measured from the moment the goals are read.
const goalsOf = (n) => ({
  '01': [goalOf('RUNNING', new Date(Date.now() + 3_600_000 + 20_000).toISOString(), null)],
  '02': [goalOf('BREACHED', null, new Date(Date.now() - 18 * 60_000).toISOString())],
  '03': [goalOf('PAUSED', null, null)],
})[n];
/** Each queue's answer, by its narrowing: what the server would hold for this reader. */
const QUEUES = {
  mine: [ticket('01', 'The printer on the third floor prints every page twice and nobody knows why')],
  unassigned: [ticket('02', 'VPN drops at noon'), ticket('03', 'New laptop for Ana')],
  breaching: [],
  open: [ticket('01', 'The printer on the third floor prints every page twice and nobody knows why'), ticket('02', 'VPN drops at noon'), ticket('03', 'New laptop for Ana')],
};
function queueOf(body) {
  const nodes = body?.filter?.nodes ?? [];
  const narrowing = nodes[2];
  if (!narrowing) return 'open';
  if (narrowing.field === 'sla_standing') return 'breaching';
  return narrowing.op === 'IS_NULL' ? 'unassigned' : 'mine';
}
const queries = [];
const OTHER = { id: '01a0e2e0-0000-7000-8000-000000000999', display_name: 'Mira Holm' };
const RECENT = [{ ...ticket('02', 'VPN drops at noon'), updated_at: '2026-11-04T09:30:00Z' }, { ...ticket('03', 'New laptop for Ana'), updated_at: '2026-11-03T08:00:00Z' }];
const releaseOf = (n, name, on, state = 'PLANNED') => ({ id: `01a0e2e0-0000-7000-8000-0000000007${n}`, hub_id: HUB.id, name, description: '', release_on: on, state, released_at: null, notes_page_id: null, version: 1 });
const RELEASES = [releaseOf('01', 'Winter release with the new planning board and the reworked reports', '2026-12-10'), releaseOf('02', 'Autumn fixes', '2026-11-20'), releaseOf('03', 'Summer', '2026-07-01', 'RELEASED')];
const PORTFOLIOS = [{ id: '01a0e2e0-0000-7000-8000-000000000811', parent_id: null, name: 'Wachstum im Norden', description: '', owner_id: null, status: 'ON_TRACK', items: [], progress: 0, version: 1 }];
const DASHBOARDS = [{ id: '01a0e2e0-0000-7000-8000-000000000821', hub_id: null, owner_id: ACCOUNT.id, name: 'Betrieb am Morgen', sharing: 'PRIVATE', widgets: [], version: 1 }];
const SPRINTS = [{ id: '01a0e2e0-0000-7000-8000-000000000831', collection_id: SCRUM.id, name: 'Sprint Kranich', goal: '', start_on: '2026-11-02', end_on: '2026-11-13', state: 'ACTIVE', unit: 'POINTS', committed_value: 0, committed_count: 0, completed_value: 0, completed_count: 0, started_at: null, closed_at: null, version: 1 }];
const GOAL_ROWS = [{ id: '01a0e2e0-0000-7000-8000-000000000801', period_id: 'p1', hub_id: null, parent_id: null, title: 'Mehr Kundinnen gewinnen', description: '', owner_id: null, status: 'ON_TRACK', confidence: null, grade: null, key_results: [], entries: [], child_count: 0, progress: 0, version: 1 }];
const CIS = [{ id: '01a0e2e0-0000-7000-8000-000000000901', hub_id: HUB.id, class: 'firewall', name: 'FortiGate 60F', stage: 'LIVE', status: 'OK', attributes: {}, provenance: {}, protection: null, quantity: 1, created_at: '2026-10-01T00:00:00Z', updated_at: '2026-10-01T00:00:00Z', version: 1 }];
const BOARDS = [board('01', 'Retro of the autumn release with every team that took part in it', '2026-10-30T09:00:00Z'), board('02', 'Kick-off', '2026-10-01T09:00:00Z')];

/** The desk's workflow, where a test runs one: the desk's own list groups its requests by status. */
const DESK_FLOW = {
  key: 'incident_flow', name: 'Incident', system: false, hub_id: null, version: 1,
  published: {
    initial: 'new',
    statuses: [
      { key: 'new', name: 'New', category: 'TODO' },
      { key: 'working', name: 'Working', category: 'IN_PROGRESS' },
      { key: 'solved', name: 'Solved', category: 'DONE' },
    ],
    steps: [{ key: 'take', name: 'Take', from: ['new'], to: 'working' }, { key: 'solve', name: 'Solve', from: ['working'], to: 'solved' }],
  },
};
/** The desk as a desk's template makes it: requests with their parts. */
const LEVELLED_DESK = { ...DESK, structure: { settings: { ...DESK.structure.settings, levels: [{ type: 'TASK', name: null }, { type: 'WORK_PACKAGE', name: null }] } } };
const STATUS_OF = { '01': 'working', '02': 'new', '03': 'new' };

/** What `/search:all` finds, each hit with the words that find it. */
const found = (kind, id, title, words, more = {}) => ({ kind, id, title, words, hub_id: HUB.id, collection_id: null, tile: null, snippet: null, state: null, key: null, updated_at: '2026-11-01T00:00:00Z', created_at: '2026-10-01T00:00:00Z', author_id: null, on: null, ...more });
const FOUND = [
  found('PAGE', PAGES[0].id, PAGES[0].title, ['release'], { snippet: 'Your first «release» week' }),
  found('WHITEBOARD', BOARDS[0].id, BOARDS[0].title, ['release']),
  found('WHITEBOARD', BOARDS[1].id, BOARDS[1].title, ['kick-off']),
  found('GOAL', GOAL_ROWS[0].id, GOAL_ROWS[0].title, ['kundinnen'], { hub_id: null }),
  found('ASSET', CIS[0].id, CIS[0].name, ['fortigate']),
  found('DASHBOARD', DASHBOARDS[0].id, DASHBOARDS[0].name, ['morgen'], { hub_id: null }),
  found('PORTFOLIO', '01a0e2e0-0000-7000-8000-000000000a01', 'Autumn programme', ['programme'], { hub_id: null, state: 'ON_TRACK' }),
  found('RELEASE', RELEASES[1].id, RELEASES[1].name, ['fixes'], { state: 'PLANNED' }),
  found('SPRINT', '01a0e2e0-0000-7000-8000-000000000a02', 'Sprint 24', ['sprint 24'], { collection_id: SCRUM.id, state: 'ACTIVE' }),
];
/** The API at the network edge, with the workspace's modules and the hub's own. */
function stubFor({ modulesOn, hubModules, deskless = false, role, deskFlow = false }) {
  return async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path.endsWith('/api/v1/stream')) return route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'retry: 3600000\n\n' });
    if (path.endsWith('/api/v1/sync:snapshot')) {
      const record = JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: HUB.id, container_id: null, payload: HUB });
      return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${record}\n{"cursor":"c-e2e"}\n` });
    }
    if (path.endsWith('/api/v1/sync:pull')) return route.fulfill({ json: { changes: [], cursor: 'c-e2e', has_more: false, tombstone_window_days: 90 } });
    if (path.endsWith('/api/v1/accounts/me')) return route.fulfill({ json: ACCOUNT });
    if (path.endsWith('/api/v1/meta/capabilities')) {
      return route.fulfill({ json: { product_version: '0.9.0', api_version: 'v1', tenancy_mode: 'single', item_types: deskFlow ? MANIFEST.item_types : [], supported_locales: [{ locale: 'en', direction: 'ltr' }], workspace_permissions: PERMISSIONS.OWNER, modules_on: modulesOn, ...(role ? { roles: ROLES } : {}) } });
    }
    if (path.endsWith(`/api/v1/containers/${HUB.id}/modules`)) return route.fulfill({ json: { modules_on: hubModules ?? modulesOn } });
    if (path.endsWith('/api/v1/agents')) return route.fulfill({ json: { data: AGENTS } });
    if (path.endsWith('/api/v1/agent-requests')) return route.fulfill({ json: { data: [HELD] } });
    if (path.endsWith(`/api/v1/agents/${AGENTS[1].id}/jobs`)) return route.fulfill({ json: { data: JOBS } });
    if (/\/api\/v1\/agents\/[^/]+\/jobs$/.test(path)) return route.fulfill({ json: { data: [] } });
    if (path.endsWith(`/api/v1/hubs/${HUB.id}/whiteboards`)) return route.fulfill({ json: { data: BOARDS } });
    if (path.endsWith('/api/v1/goals')) return route.fulfill({ json: { data: GOAL_ROWS } });
    if (path.endsWith('/api/v1/portfolios')) return route.fulfill({ json: { data: PORTFOLIOS } });
    if (path.endsWith('/api/v1/dashboards')) return route.fulfill({ json: { data: DASHBOARDS } });
    if (path.endsWith(`/api/v1/collections/${SCRUM.id}/sprints`)) return route.fulfill({ json: { data: SPRINTS, runs_sprints: true } });
    if (/\/api\/v1\/collections\/[^/]+\/sprints$/.test(path)) return route.fulfill({ json: { data: [], runs_sprints: false } });
    if (path.endsWith(`/api/v1/hubs/${HUB.id}/cis`)) return route.fulfill({ json: { data: CIS, next_after: null } });
    if (path.endsWith(`/api/v1/hubs/${HUB.id}/pages`) && !url.searchParams.has('templates')) return route.fulfill({ json: { data: PAGES } });
    if (path.endsWith(`/api/v1/hubs/${HUB.id}/cmdb-health`)) return route.fulfill({ json: HEALTH });
    if (path.endsWith(`/api/v1/hubs/${HUB.id}/releases`)) return route.fulfill({ json: { data: RELEASES } });
    if (path.endsWith('/api/v1/search:all')) {
      const { q, kinds } = route.request().postDataJSON();
      const data = FOUND.filter((hit) => kinds.includes(hit.kind) && hit.words.some((word) => q.toLowerCase().includes(word))).map(({ words, ...hit }) => hit);
      const facets = kinds.map((kind) => ({ kind, count: data.filter((hit) => hit.kind === kind).length, more: false }));
      return route.fulfill({ json: { data, facets } });
    }
    if (path.endsWith('/api/v1/pages:search')) return route.fulfill({ json: { data: [{ id: PAGES[0].id, hub_id: HUB.id, parent_id: null, title: 'Onboarding guide', icon: null, excerpt: 'Your first «release» week', rank: 1, updated_at: PAGES[0].updated_at }] } });
    if (path.endsWith('/api/v1/search')) return route.fulfill({ json: { data: [], page: { next_cursor: null, has_more: false } } });
    if (path.endsWith('/api/v1/deadlines-kept')) return route.fulfill({ json: { from: '2026-10-14T00:00:00Z', to: '2026-11-13T00:00:00Z', data: KEPT } });
    // The server names the hub's desks; a deskless workspace has none.
    if (path.endsWith(`/api/v1/hubs/${HUB.id}/queues`)) return route.fulfill({ json: { data: [], desks: deskless ? [] : [DESK.id] } });
    if (path.endsWith('/api/v1/entry-types')) return route.fulfill({ json: { data: deskless ? TYPES.map(({ form, ...type }) => type) : TYPES } });
    if (role && path.endsWith('/api/v1/memberships')) {
      const rows = url.searchParams.get('scope_type') === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role }] : [];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: [] } });
    }
    const sla = /\/api\/v1\/items\/01a0e2e0-0000-7000-8000-0000000006(\d\d)\/sla$/.exec(path);
    if (sla) return route.fulfill({ json: { data: goalsOf(sla[1]) ?? [] } });
    if (path.endsWith('/api/v1/accounts:resolve')) return route.fulfill({ json: { data: [{ id: ACCOUNT.id, display_name: ACCOUNT.display_name }, OTHER] } });
    const history = /\/api\/v1\/items\/([^/]+)\/activity$/.exec(path);
    if (history) {
      const at = RECENT.find((entry) => entry.id === history[1]);
      // The first entry was made, the second moved to a status by a step: its sentence names it.
      const stepped = at === RECENT[1];
      return route.fulfill({ json: { ...PAGE, data: at ? [{
        id: `s-${at.id}`, item_id: at.id, code: stepped ? 'activity.item_step_taken' : 'activity.item_created', actor: { type: 'USER', id: OTHER.id }, occurred_at: at.updated_at,
        change_set: stepped ? { step: { to: 'start' }, status: { from: 'todo', to: 'in_progress' } } : {},
      }] : [] } });
    }
    // The hub's plan in two phases, the first done; the sprint's burndown over three days.
    if (/\/api\/v1\/plans\/[^/]+\/schedule$/.test(path)) {
      const phase = (id, title, start, finish, done) => ({ item_id: id, title, parent_id: null, collection_id: SCRUM.id, start, finish, duration_days: 5, moved: false, total_float_days: 0, free_float_days: 0, critical: false, summary: true, external: false, mode: 'AUTOMATIC', milestone: false, done });
      return route.fulfill({ json: { zone: 'UTC', finish: '2026-12-05', entries: [phase('p1', 'Groundwork', '2026-10-01', '2026-10-31', true), phase('p2', 'Shell', '2026-11-01', '2026-12-05', false)], dependencies: [], conflicts: [], unscheduled: [], critical_path: [] } });
    }
    if (/\/api\/v1\/sprints\/[^/]+$/.test(path)) {
      return route.fulfill({ json: { sprint: SPRINTS[0], entries: [], burndown: [{ day: '2026-11-02', remaining: 21, scope: 21, ideal: 21 }, { day: '2026-11-03', remaining: 12, scope: 21, ideal: 14 }] } });
    }
    // The hub's plan lies four days past its baseline, as the query reads it by collection - no
    // plan's tolerance is read, since that read records an exception.
    if (/\/api\/v1\/plans\/[^/]+\/tolerance$/.test(path)) return route.abort();
    if (path.endsWith('/api/v1/items:aggregate')) {
      const body = route.request().postDataJSON();
      if (body.aggregate?.field === 'slip_days') return route.fulfill({ json: { data: [{ group: SCRUM.id, period: null, count: 2, value: 4 }], truncated: false } });
      if (body.q === 'is: open') return route.fulfill({ json: { data: [{ group: SCRUM.id, period: null, count: 7, value: null }], truncated: false } });
    }
    // A release's entries: the autumn one waits for one entry, the winter one is four of ten.
    if (/\/api\/v1\/releases\/[^/]+$/.test(path)) {
      const release = RELEASES.find((each) => path.endsWith(each.id));
      const entryOf = (n, done) => ({ item_id: `e${n}`, title: `Entry ${n}`, type_name: '', done, collection_id: SCRUM.id, added_at: '2026-11-01T09:00:00Z' });
      const entries = release === RELEASES[1] ? [entryOf(1, true), entryOf(2, false)] : [...Array.from({ length: 4 }, (_, n) => entryOf(n, true)), ...Array.from({ length: 6 }, (_, n) => entryOf(n + 4, false))];
      return route.fulfill({ json: { ...release, entries, done_count: entries.filter((each) => each.done).length, open_count: entries.filter((each) => !each.done).length } });
    }
    if (path.endsWith('/api/v1/sla-policies')) return route.fulfill({ json: { data: [] } });
    if (path.endsWith('/api/v1/time-records:report')) return route.fulfill({ json: { group_by: [], groups: [], minutes: 390, withheld_minutes: 0, withheld_groups: 0 } });
    if (path.endsWith('/api/v1/items:query')) {
      const body = route.request().postDataJSON();
      queries.push(body);
      if (body?.sort?.[0]?.field === 'updated_at') return route.fulfill({ json: { data: RECENT, groups: [], page: { next_cursor: null, has_more: false }, total: null } });
      const queued = QUEUES[queueOf(body)];
      const rows = deskFlow ? queued.map((row) => ({ ...row, status: STATUS_OF[row.id.slice(-2)] })) : queued;
      return route.fulfill({ json: { data: rows, groups: [], page: { next_cursor: null, has_more: false }, total: rows.length } });
    }
    if (deskFlow && path.endsWith('/api/v1/workflows')) return route.fulfill({ json: { data: [DESK_FLOW], scheme: [{ container_id: DESK.id, entry_type: null, workflow: DESK_FLOW.key }] } });
    if (path.endsWith('/api/v1/containers') && url.searchParams.get('parent_id') === HUB.id) return route.fulfill({ json: { ...PAGE, data: [deskFlow ? LEVELLED_DESK : DESK, SCRUM] } });
    if (path.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: [HUB] } });
    if (path.endsWith('/api/v1/containers')) return route.fulfill({ json: { ...PAGE, data: [] } });
    if (/\/(labels|buckets|views|templates|custom-fields|policies|members)$/.test(path)) return route.fulfill({ json: [] });
    return fallback(route, route.request(), path);
  };
}

const LINE = 'main [data-head="search"] input[role="combobox"]';
const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, { width, scheme = 'light', path = '/', reducedMotion = 'no-preference', ...modules }) {
  const context = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: scheme, reducedMotion });
  await context.route('**/api/v1/**', stubFor(modules));
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}${path}`);
  // A phone's deeper screen has the bar's ‹ where a root has the avatar (the navigation concept §9).
  await page.getByRole('button', { name: ACCOUNT.display_name }).or(page.locator('[data-phone-bar] [data-back]')).first().waitFor({ timeout: 15_000 });
  return { page, failures, close: () => context.close() };
}

async function noSidewaysScroll(page, label) {
  const scroll = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: window.innerWidth }));
  assert.equal(scroll.width, scroll.viewport, `${label}: the page scrolls sideways (${scroll.width} of ${scroll.viewport})`);
}

// The colour scheme changes no row or box here: one scheme per width.
for (const scheme of ['light']) {
  test(`chromium 1280 px, ${scheme}: the products section, in one order, and a product's home with its hubs`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures, close } = await open(browser, { width: 1280, scheme, modulesOn: ['TIME', 'SERVICE_DESK', 'KNOWLEDGE'] });
    t.after(close);
    const side = page.locator('aside.sidenav');
    await side.getByRole('tree', { name: 'Hubs' }).getByRole('treeitem', { name: new RegExp(HUB.name) }).waitFor({ timeout: 10_000 });
    const first = (texts) => texts.map((row) => row.split('\n')[0].trim());
    // For you (the approvals where the desk is on), its count beside *Waiting for your yes*; then
    // the products in Module.Keys() order whatever order the manifest answers, then the hubs.
    const top = side.getByRole('list', { name: 'For you' });
    assert.deepEqual(first(await top.getByRole('link').allInnerTexts()), ['Today', 'Inbox', 'Waiting for your yes']);
    await top.getByRole('link', { name: /Waiting for your yes/ }).filter({ hasText: '1' }).waitFor({ timeout: 5_000 });
    const products = side.getByRole('region', { name: 'Products' });
    // The products are a tree whose rows are dragged into order (UC-WRK-38 check 7).
    assert.deepEqual(first(await products.getByRole('treeitem').allInnerTexts()), ['Pages', 'Service desk', 'Time']);
    assert.deepEqual(first(await side.getByRole('tree', { name: 'Hubs' }).getByRole('treeitem').allInnerTexts()), [HUB.name]);
    assert.equal(await side.getByText('Products', { exact: true }).count(), 1, 'the section has no caption');
    const tree = products;
    // The account menu carries none of them.
    await page.getByRole('button', { name: ACCOUNT.display_name }).click();
    const items = (await page.getByRole('menu').getByRole('menuitem').allTextContents()).map((item) => item.trim());
    for (const product of ['Pages', 'Service desk', 'Time', 'My notes', 'Waiting for your yes', 'Administration']) assert.ok(!items.includes(product), `${product} is behind the avatar`);
    await page.keyboard.press('Escape');

    await tree.getByRole('treeitem', { name: 'Pages' }).click();
    await page.waitForFunction(() => location.pathname === '/pages', null, { timeout: 5_000 });
    assert.equal((await page.getByRole('heading', { level: 1 }).textContent()).trim(), 'Pages');
    assert.equal(await tree.getByRole('treeitem', { name: 'Pages' }).getAttribute('aria-current'), 'page', 'the row is not current on its home');
    const tabs = page.getByRole('main').getByRole('navigation', { name: 'Pages' });
    // The shared vocabulary (the navigation concept §4): *For you* first, the spaces a tab of their own.
    assert.deepEqual(first(await tabs.getByRole('link').allInnerTexts()), ['For you', 'Spaces', 'My notes']);
    assert.equal(await tabs.getByRole('link', { name: 'For you' }).getAttribute('aria-current'), 'page');
    await tabs.getByRole('link', { name: 'Spaces' }).click();
    await page.waitForFunction(() => location.search === '?tab=spaces', null, { timeout: 5_000 });
    assert.equal(await tabs.getByRole('link', { name: 'Spaces' }).getAttribute('aria-current'), 'page');
    // The hub, one line with the whole name as its tooltip, opening the hub's pages.
    const hub = page.getByRole('main').getByRole('link', { name: HUB.name });
    await hub.waitFor({ timeout: 5_000 });
    assert.equal(await hub.getAttribute('href'), `/hubs/${HUB.id}/pages`);
    await noSidewaysScroll(page, `1280 ${scheme}`);
    // *My notes* is the home's second tab, at its own address, and the row stays current.
    await tabs.getByRole('link', { name: 'My notes' }).click();
    await page.waitForFunction(() => location.pathname === '/notes', null, { timeout: 5_000 });
    assert.equal(await tree.getByRole('treeitem', { name: 'Pages' }).getAttribute('aria-current'), 'page');
    assert.deepEqual(failures, []);
  });
}

test('chromium 1280 px: a product no hub uses says so and offers a hub; with nothing on there is no band', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const narrowed = await open(browser, { width: 1280, path: '/whiteboards', modulesOn: ['WHITEBOARD'], hubModules: [] });
  t.after(narrowed.close);
  await narrowed.page.getByText('No hub uses Whiteboards yet').waitFor({ timeout: 10_000 });
  assert.equal(await narrowed.page.getByRole('main').getByRole('button', { name: 'Create hub' }).count(), 1, 'no way forward from the empty home');

  const none = await open(browser, { width: 1280, path: '/whiteboards', modulesOn: [] });
  t.after(none.close);
  await none.page.locator('aside.sidenav').getByRole('tree', { name: 'Hubs' }).getByRole('treeitem', { name: new RegExp(HUB.name) }).waitFor({ timeout: 10_000 });
  assert.equal(await none.page.locator('aside.sidenav').getByText('Products', { exact: true }).count(), 0, 'a household with nothing on sees the caption');
  // Reached by a link while off: the neutral screen with its way forward (§2.8).
  await none.page.getByRole('button', { name: 'Switch Whiteboards on' }).waitFor({ timeout: 5_000 });
  assert.equal(await none.page.getByText('Products', { exact: true }).count() > 0, true, 'the trail is not the products');
  assert.deepEqual([...narrowed.failures, ...none.failures], []);
});

for (const scheme of ['dark']) {
  test(`chromium 375 px, ${scheme}: Browse lists the products, the hub and the foot`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures, close } = await open(browser, { width: 375, scheme, modulesOn: ['SERVICE_DESK', 'ASSETS'] });
    t.after(close);
    // Today with *More in Hubtask* fits the phone; on a phone it stands in the overview half.
    await page.getByRole('tab', { name: 'Overview' }).click({ timeout: 10_000 });
    await page.getByRole('region', { name: 'More in Hubtask' }).waitFor({ timeout: 10_000 });
    await noSidewaysScroll(page, `Today 375 ${scheme}`);
    const bar = page.getByRole('navigation', { name: 'Sections' });
    await bar.getByRole('link', { name: 'Browse' }).click();
    const browse = page.locator('[data-browse-step]');
    await browse.getByRole('link', { name: new RegExp(HUB.name) }).waitFor({ timeout: 5_000 });
    const rows = (await browse.locator('a').allInnerTexts()).map((row) => row.split('\n')[0].trim());
    // What the bottom bar does not carry: the approvals, then the products, the hub, the
    // administration and the keeping (the navigation concept §9).
    assert.deepEqual(rows, ['Waiting for your yes', 'Service desk', 'Assets', HUB.name, 'Administration', 'Archive', 'Trash'], JSON.stringify(rows));
    await browse.getByRole('link', { name: 'Assets' }).click();
    await page.waitForFunction(() => location.pathname === '/assets', null, { timeout: 5_000 });
    assert.equal(await bar.getByRole('link', { name: 'Browse' }).getAttribute('aria-current'), 'page', 'Browse is not current on a product home');
    await page.getByRole('main').getByRole('link', { name: HUB.name }).waitFor({ timeout: 5_000 });
    await noSidewaysScroll(page, `375 ${scheme}`);
    assert.deepEqual(failures, []);
  });
}

// user-experience.md §3.3: *More in Hubtask* offers the products that are off to whoever may switch
// one on, each with what it does; *Try it* opens its home with the switch; it can be hidden for good.
test('chromium 1280 px: Today offers the products that are off, and hides them for good', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, modulesOn: ['KNOWLEDGE'] });
  t.after(close);
  const more = page.getByRole('region', { name: 'More in Hubtask' });
  await more.waitFor({ timeout: 10_000 });
  const names = (await more.locator('.offer-name').allTextContents()).map((name) => name.trim());
  assert.deepEqual(names, ['Plans', 'Whiteboards', 'Service desk', 'Assets', 'Time', 'Goals', 'Agents'], JSON.stringify(names));
  await more.getByRole('listitem').filter({ hasText: 'Whiteboards' }).getByRole('button', { name: 'Try it' }).click();
  await page.waitForFunction(() => location.pathname === '/whiteboards', null, { timeout: 5_000 });
  await page.getByRole('button', { name: 'Switch Whiteboards on' }).waitFor({ timeout: 5_000 });
  await page.goto(`${served.origin}/`);
  await more.getByRole('button', { name: 'Hide More in Hubtask for good' }).click();
  await more.waitFor({ state: 'detached', timeout: 5_000 });
  await page.reload();
  await page.getByRole('button', { name: ACCOUNT.display_name }).waitFor({ timeout: 15_000 });
  await page.waitForTimeout(500);
  assert.equal(await more.count(), 0, 'More in Hubtask came back after it was hidden');
  assert.deepEqual(failures, []);
});

// §2.2: the whiteboards' home lists the boards themselves, recent first, as cards.
for (const [width, scheme] of [[1280, 'light'], [375, 'dark']]) {
  test(`chromium ${width} px, ${scheme}: the whiteboards' home shows the boards, the last changed first`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures, close } = await open(browser, { width, scheme, path: '/whiteboards', modulesOn: ['WHITEBOARD'] });
    t.after(close);
    const recent = page.getByRole('region', { name: 'Recently changed boards' });
    await recent.waitFor({ timeout: 10_000 });
    const titles = await recent.getByRole('link').evaluateAll((links) => links.map((link) => link.querySelector('[title]')?.getAttribute('title')));
    assert.deepEqual(titles, [BOARDS[0].title, BOARDS[1].title]);
    assert.equal(await recent.getByRole('link').first().getAttribute('href'), `/whiteboards/${BOARDS[0].id}`);
    await noSidewaysScroll(page, `whiteboards ${width} ${scheme}`);
    assert.deepEqual(failures, []);
  });
}

// §7.8, page-system §9.5: the agents' home is their list, live first and a retired one last, each
// row counting the acts waiting on it; the runs are a tab, the newest first.
test('chromium 1280 px: the agents\' home lists the agents, their held acts and runs', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: '/agents', modulesOn: ['AGENTS'] });
  t.after(close);
  const names = page.locator('[data-agent-row]');
  await names.first().waitFor({ timeout: 10_000 });
  assert.deepEqual(await names.allTextContents(), ['Request sorter', 'Duplicate finder', 'Old sorter']);
  // What waits for the reader stands first (scene homeagents): the held act with its two answers.
  const waiting = page.locator('[data-agents-waiting]');
  await waiting.locator('[data-held-act]').first().waitFor({ timeout: 10_000 });
  assert.equal(await waiting.locator('[data-held-act]').count(), 1);
  assert.ok(await waiting.evaluate((section) => section.compareDocumentPosition(document.querySelector('[data-agent-card]')) & Node.DOCUMENT_POSITION_FOLLOWING), 'the agents come after what waits');
  const first = page.locator('[data-agent-card]').filter({ hasText: 'Request sorter' });
  assert.match((await first.textContent()) ?? '', /Live/u);
  await first.getByRole('button', { name: '1 waiting' }).click();
  await page.waitForURL(new RegExp(`/agents\\?tab=requests&agent=${AGENTS[1].id}$`));
  await page.getByText('Waiting on Request sorter').waitFor({ timeout: 10_000 });
  await page.getByRole('main').getByRole('link', { name: /^Runs/ }).click();
  const runs = page.getByRole('table', { name: 'Runs' });
  await runs.waitFor({ timeout: 10_000 });
  assert.match((await runs.textContent()) ?? '', /Done[\s\S]*Failed/u, 'the newest run first');
  assert.deepEqual(failures, []);
});

// §2.2: the pages' home lists the reader's pages to review and the recently changed ones, the
// space's content only; the assets' home opens with the inventory's health across hubs.
for (const [width, scheme] of [[1280, 'dark'], [375, 'light']]) {
  test(`chromium ${width} px, ${scheme}: the pages' home shows what waits for review and what changed`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures, close } = await open(browser, { width, scheme, path: '/pages', modulesOn: ['KNOWLEDGE'] });
    t.after(close);
    const review = page.getByRole('region', { name: 'Waiting for your review' });
    await review.waitFor({ timeout: 10_000 });
    assert.deepEqual(await review.locator('.name').allTextContents(), ['Support rota']);
    const recent = page.getByRole('region', { name: 'Recently changed pages' });
    assert.deepEqual(await recent.locator('.name').allTextContents(), [PAGES[1].title, 'Onboarding guide', 'Support rota']);
    assert.equal(await recent.getByRole('link').first().getAttribute('href'), `/pages/${PAGES[1].id}`);
    await noSidewaysScroll(page, `pages ${width} ${scheme}`);
    assert.deepEqual(failures, []);
  });

  test(`chromium ${width} px, ${scheme}: the assets' home opens with the inventory's health`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures, close } = await open(browser, { width, scheme, path: '/assets', modulesOn: ['ASSETS'] });
    t.after(close);
    const inventory = page.getByRole('region', { name: 'Inventory across your hubs' });
    await inventory.waitFor({ timeout: 10_000 });
    const figures = await inventory.locator('dl > div').evaluateAll((rows) => rows.map((row) => `${row.querySelector('dt')?.textContent}=${row.querySelector('dd')?.textContent}`));
    assert.deepEqual(figures, ['Configuration items=42', 'Need attention=2', 'Review overdue=4']);
    // The inventory itself across hubs, by class, the most held first, each saying where (§2.2).
    const byClass = page.getByRole('region', { name: 'By class' });
    await byClass.waitFor({ timeout: 5_000 });
    assert.deepEqual(await byClass.locator('.name').allTextContents(), ['Server · 30', 'Laptop · 12']);
    assert.equal(await byClass.locator('.purpose').first().innerText(), `30 in ${HUB.name}`);
    assert.equal(await byClass.getByRole('link').first().getAttribute('href'), `/hubs/${HUB.id}/assets`);
    // What makes one need attention is said beside the figure, not left to guess.
    await inventory.getByText('A configuration item needs attention when it is incomplete').waitFor({ timeout: 5_000 });
    await noSidewaysScroll(page, `assets ${width} ${scheme}`);
    assert.deepEqual(failures, []);
  });
}

// §2.2, §2.4, §7.4: the desk's home and a hub's Desk tab draw the handler's queues across the
// hub's desk collections only, with their counts, what breaks first on top (UC-SVC-03).
for (const [width, scheme] of [[1280, 'light'], [375, 'dark']]) {
  test(`chromium ${width} px, ${scheme}: the desk's home and the hub's Queues draw the queues`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    queries.length = 0;
    const { page, failures, close } = await open(browser, { width, scheme, path: '/desk', modulesOn: ['WORKFLOWS', 'SERVICE_DESK'] });
    t.after(close);
    // The queue is the filter bar's first chip, pinned beside the search on a phone too; its counts
    // in the popover (a sheet on a phone).
    const chip = page.getByRole('button', { name: 'Queue: Mine (1)' });
    await chip.waitFor({ timeout: 10_000 });
    assert.equal(await page.getByRole('button', { name: /^Filters/u }).count(), 0, 'the queue hides behind Filters');
    await chip.click();
    const queues = page.locator('[data-desk-queues]');
    await queues.getByRole('radio', { name: 'Mine (1)' }).waitFor({ timeout: 10_000 });
    assert.deepEqual((await queues.getByRole('radio').evaluateAll((nodes) => nodes.map((node) => node.labels?.[0]?.textContent ?? ''))).map((each) => each.trim()), ['Mine (1)', 'Unassigned (2)', 'About to breach (0)', 'All open (3)']);
    // Only the desk is asked, by what breaks first, counted.
    assert.ok(queries.length > 0 && queries.every((body) => JSON.stringify(body.filter.nodes[0]) === JSON.stringify({ op: 'IN', field: 'collection_id', value: [DESK.id] }) && body.sort[0].field === 'sla_due' && body.count === 'exact'), JSON.stringify(queries));
    // §7.4: the open requests as a table, the SLA said in words beside its bar, never by colour alone.
    const panel = page.getByRole('region', { name: 'All open', exact: true });
    await queues.getByRole('radio', { name: 'All open (3)' }).click();
    // On a phone the sheet stays open over the list until it is shown; the popover closes.
    if (width >= 600) {
      await page.keyboard.press('Escape');
      await page.getByRole('button', { name: 'Queue: All open (3)' }).waitFor({ timeout: 5_000 });
    }
    const table = panel.getByRole('table', { name: 'Requests in All open' });
    await table.locator('[data-sla="breached"] >> visible=true').first().waitFor({ timeout: 10_000 });
    const headings = await table.locator('thead th').allTextContents();
    // The calm table (WB-4, scene queue): the status is the group, each with its count and "+ request".
    if (width >= 600) assert.deepEqual(headings.map((each) => each.trim()).filter(Boolean), ['Key', 'Request', 'Type', 'Requester', 'Priority', 'SLA', 'Assignee']);
    const cells = await table.locator('tbody tr.row').evaluateAll((rows) => rows.map((row) => [...row.querySelectorAll('td')].filter((cell) => !cell.classList.contains('select')).map((cell) => cell.textContent.trim().replace(/\s+/g, ' '))));
    if (width >= 600) assert.deepEqual(cells.map((row) => [row[0], row[3], row[4], row[5]]), [
      ['INC-1', ACCOUNT.display_name, 'P2', '1 h 0 min left'], ['INC-2', ACCOUNT.display_name, 'P3', 'Breached 18 min ago'], ['INC-3', ACCOUNT.display_name, 'P4', 'Paused'],
    ], JSON.stringify(cells));
    // UC-SVC-02 check 15 (the owner's decision of 2026-11-12): my deadlines kept, every kind worst
    // first, three rows then "+ n more"; the phone draws only the worst.
    const kept = page.locator('[data-desk-kept]');
    assert.deepEqual((await kept.locator('[data-kept-row] >> visible=true').allInnerTexts()).map((each) => each.replace(/\s+/g, ' ').trim()),
      width >= 600 ? ['A team: Resolution 33 % 2 breached', 'First response 50 % 1 breached', 'Next response 80 % 1 breached'] : ['A team: Resolution 33 % 2 breached']);
    if (width >= 600) {
      await kept.getByRole('button', { name: '+ 1 more' }).click();
      const dialog = page.getByRole('dialog', { name: 'My deadlines kept' });
      assert.equal(await dialog.locator('[data-kept-row]').count(), 4, 'the whole table of kinds');
      await page.keyboard.press('Escape');
      await dialog.waitFor({ state: 'hidden' });
    }
    // The band's figures (E-1, WB-5): the first one is the headline.
    assert.deepEqual((await page.locator('.band .figure').allTextContents()).map((each) => each.trim()), ['3 open', '2 unassigned']);
    // UC-SVC-03 check 12, UC-WRK-45 check 9: grouped by status, each group counted, each ending in
    // capture's door for the desk; on a phone the key, the status and the priority under the title.
    const counts = await table.locator('tr.group-row[data-head] .group-count').allTextContents();
    assert.equal(counts.reduce((sum, each) => sum + Number(each.trim()), 0), 3, `the groups count ${counts}`);
    assert.ok(await table.getByRole('button', { name: /^Request in / }).count() >= 1, 'no "+ Request in" at a group\'s end');
    if (width < 600) {
      const meta = (await table.locator('td.stack .meta').first().textContent()).replace(/\s+/g, ' ');
      assert.ok(/INC-\d/.test(meta) && /P\d/.test(meta), `the meta line says ${meta}`);
    }
    // A request's title is read whole, on a phone's stacked card too, not cut to a few letters.
    const title = await table.locator('.title').first().evaluate((node) => [node.scrollWidth, node.clientWidth]);
    if (width < 600) assert.ok(title[0] <= title[1] + 1 && title[1] >= 150, `the title is cut: ${title}`);
    // Scene queue: at a desktop's width the request column is the title's; the row's actions, seen
    // on hover, lie over its end rather than keep a room of their own beside it.
    if (width >= 600) {
      const room = await table.locator('tbody tr.row td[data-role="title"]').evaluateAll((cells) => cells.map((cell) => {
        const title = cell.querySelector('.title');
        const acts = cell.querySelector('.end-inner');
        const style = getComputedStyle(cell);
        const inner = cell.clientWidth - parseFloat(style.paddingInlineStart) - parseFloat(style.paddingInlineEnd);
        return { cut: title.scrollWidth > title.clientWidth + 1, share: title.clientWidth / inner, acts: acts ? getComputedStyle(acts).position : 'none' };
      }));
      assert.ok(room.every((each) => (!each.cut || each.share > 0.85) && each.acts !== 'static'), JSON.stringify(room));
    }
    // The SLA bar's track stands apart from the row it is drawn on, in dark mode too.
    const track = await table.locator('.sla .track >> visible=true').first().evaluate((node) => {
      const channels = (colour) => (colour.match(/[\d.]+/g) ?? []).slice(0, 3).map(Number);
      const lightness = ([r, g, b]) => (Math.max(r, g, b) + Math.min(r, g, b)) / 2;
      let behind = node.parentElement;
      while (behind && getComputedStyle(behind).backgroundColor.replace(/\s/g, '') === 'rgba(0,0,0,0)') behind = behind.parentElement;
      return [lightness(channels(getComputedStyle(node).backgroundColor)), lightness(channels(getComputedStyle(behind ?? document.body).backgroundColor))];
    });
    assert.ok(Math.abs(track[0] - track[1]) > 8, `the SLA track does not stand apart from its row: ${track}`);
    // All seven columns fit beside the tree: none hides behind the edge fade.
    const frame = await table.evaluate((node) => { const scroller = node.closest('.scroll'); return [scroller.scrollWidth, scroller.clientWidth]; });
    assert.ok(frame[0] <= frame[1] + 1, `the queue table scrolls sideways: ${frame}`);
    // One strip of tabs only: the queue is the filter bar's first chip, never a second strip.
    assert.equal(await page.getByRole('tablist').count(), 0, 'a second tab strip');
    if (width >= 600) await page.getByRole('button', { name: 'Queue: All open (3)' }).click();
    await queues.getByRole('radio', { name: 'Unassigned (2)' }).click();
    await page.getByRole('region', { name: 'Unassigned', exact: true }).getByRole('link', { name: 'VPN drops at noon' }).waitFor({ timeout: 5_000 });
    await queues.getByRole('radio', { name: 'About to breach (0)' }).click();
    await page.getByRole('region', { name: 'About to breach', exact: true }).getByText('Nothing waits in this queue').waitFor({ timeout: 5_000 });
    await noSidewaysScroll(page, `desk home ${width} ${scheme}`);

    // The hub's desk is its tool *Queues* (the navigation concept, §2.4), current in its level.
    await page.goto(`${served.origin}/hubs/${HUB.id}/desk`);
    if (width >= 1280) {
      const queuesTool = page.locator('aside.sidenav').getByRole('region', { name: HUB.name }).getByRole('link', { name: 'Queues', exact: true });
      await queuesTool.waitFor({ timeout: 10_000 });
      assert.equal(await queuesTool.getAttribute('aria-current'), 'page');
    }
    // The hub's first queue opens as its table over the hub's desks (UC-SVC-03 check 10), one
    // queue a page: no choice of the four.
    await page.getByRole('main').getByRole('table').first().waitFor({ timeout: 10_000 });
    assert.equal(await page.getByRole('main').getByRole('radiogroup', { name: 'Queues' }).count(), 0, 'the queue page draws the queues\' choice');
    await noSidewaysScroll(page, `hub desk ${width} ${scheme}`);
    assert.deepEqual(failures, []);
  });
}

// UX-5, §5.5: a hub with no desk yet offers whoever shapes it the ITIL desk's plan at its *Templates and presets*.
test('chromium 1280 px: a hub with no desk offers the ITIL desk to whoever shapes it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: `/hubs/${HUB.id}/desk`, modulesOn: ['WORKFLOWS', 'SERVICE_DESK'], deskless: true, role: 'OWNER' });
  t.after(close);
  await page.getByText('This hub has no desk yet').waitFor({ timeout: 10_000 });
  await page.getByRole('button', { name: 'Start from the ITIL defaults' }).click();
  await page.waitForFunction(() => location.pathname.endsWith('/settings/template') && location.search === '?template=itil_service_desk', null, { timeout: 5_000 });
  assert.deepEqual(failures, []);
});

// A handler with nothing of their own opens on the first queue that holds a request, not on an
// empty *Mine* while requests wait unassigned.
test('chromium 1280 px: the desk opens on the first queue that holds a request', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const mine = QUEUES.mine;
  QUEUES.mine = [];
  t.after(() => { QUEUES.mine = mine; });
  const { page, failures, close } = await open(browser, { width: 1280, path: '/desk', modulesOn: ['WORKFLOWS', 'SERVICE_DESK'] });
  t.after(close);
  await page.getByRole('region', { name: 'Unassigned', exact: true }).getByRole('link', { name: 'VPN drops at noon' }).waitFor({ timeout: 10_000 });
  await page.getByRole('button', { name: 'Queue: Unassigned (2)' }).click();
  const queues = page.locator('[data-desk-queues]');
  await queues.getByRole('radio', { name: 'Unassigned (2)', checked: true }).waitFor({ timeout: 10_000 });
  await queues.getByRole('radio', { name: 'Mine (0)' }).click();
  await page.getByRole('region', { name: 'Mine', exact: true }).getByText('Nothing waits in this queue').waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});

// UC-SVC-03 check 12 (scene queue): a desk collection's own list is its queue - the key leading
// each row and its head, the queue's columns, grouped by status, each group ending in the desk's add.
test('chromium 1280 px: a desk collection\'s own list is its queue, grouped by status with the key first', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: `/collections/${DESK.id}`, modulesOn: ['WORKFLOWS', 'SERVICE_DESK'], deskFlow: true, role: 'OWNER' });
  t.after(close);
  const table = page.locator('[data-desk][data-table]');
  await table.waitFor({ timeout: 15_000 });
  await table.getByText('VPN drops at noon').first().waitFor({ timeout: 10_000 });
  const head = table.locator('.fields-head');
  assert.deepEqual(
    (await head.locator('.col-head').allTextContents()).map((each) => each.trim()),
    ['Key', 'Request', 'Type', 'Requester', 'Priority', 'SLA', 'People'],
  );
  // Grouped by status in the workflow's order; each group ends in the desk's add.
  const groups = table.locator('.group-head[data-fold]');
  await page.waitForFunction(() => document.querySelectorAll('[data-desk] .group-head[data-fold]').length >= 2, null, { timeout: 5_000 });
  const named = (await groups.locator('.group-name').allInnerTexts()).map((each) => each.trim());
  assert.deepEqual(named.slice(0, 2), ['New', 'Working'], `groups: ${named}`);
  assert.equal(await table.getByRole('button', { name: `Request in “${DESK.name}”` }).count(), await groups.count(), 'a group without the desk\'s add');
  // The key leads its row, at the key head's start.
  const keyHead = await head.locator('.key-head').boundingBox();
  const rowKey = await table.locator('.lead-key').first().boundingBox();
  assert.ok(Math.abs(keyHead.x - rowKey.x) <= 4, `the key stands at ${rowKey.x}, its head at ${keyHead.x}`);
  assert.match(await table.locator('.lead-key').first().innerText(), /^INC-\d+$/);
  await noSidewaysScroll(page, 'desk list 1280');
  assert.deepEqual(failures, []);
});

// UC-WRK-45 check 9 (scene queue 4): on a phone a request is its title, one meta line with key,
// status and priority, one SLA bar and the person at the end of the title's line - no line of
// its own for labels or ⋯, whose menu a swipe or a long press opens.
test('chromium 390 px: a desk collection\'s phone row is title, one meta line, one SLA bar and the person', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 390, path: `/collections/${DESK.id}`, modulesOn: ['WORKFLOWS', 'SERVICE_DESK'], deskFlow: true, role: 'OWNER' });
  t.after(close);
  const row = page.locator('[data-row][data-desk-phone]').filter({ hasText: 'VPN drops at noon' }).first();
  await row.waitFor({ timeout: 15_000 });
  await row.locator('.desk-sla .sla').waitFor({ timeout: 10_000 });
  const parts = await row.evaluate((element) => {
    const box = (selector) => element.querySelector(selector)?.getBoundingClientRect();
    const visible = (each) => { const rect = each.getBoundingClientRect(); return rect.width > 2 && rect.height > 2; };
    return {
      title: box('.task-row .content'),
      meta: box('.desk-meta'),
      sla: box('.desk-sla'),
      person: box('.desk-person'),
      metaText: element.querySelector('.desk-meta')?.textContent ?? '',
      bars: element.querySelectorAll('.sla').length,
      chips: element.querySelectorAll('.desk-meta [data-state-chip], .desk-meta button').length,
      shownButtons: [...element.querySelectorAll('button')].filter(visible).map((each) => each.getAttribute('aria-label') ?? each.textContent?.trim()),
      menus: element.querySelectorAll('.desk-menu button').length,
    };
  });
  assert.equal(parts.bars, 1, 'the SLA is drawn more than once');
  assert.match(parts.metaText, /INC-\d+/u);
  assert.ok(parts.meta.y >= parts.title.y + parts.title.height - 1, 'the meta line is not under the title');
  assert.ok(parts.meta.height < 40, `the meta line wraps: ${parts.meta.height}`);
  assert.ok(parts.sla.y >= parts.meta.y + parts.meta.height - 1, 'the SLA bar is not under the meta line');
  assert.ok(parts.person.x >= parts.title.x + parts.title.width - 1 && Math.abs(parts.person.y - parts.title.y) < 16, 'the person is not at the end of the title\'s line');
  assert.equal(parts.menus, 1, 'the row\'s menu is gone');
  assert.ok(!parts.shownButtons.some((label) => /labels|more|⋯/iu.test(label ?? '')), `a line of row tools is drawn: ${parts.shownButtons}`);
  await noSidewaysScroll(page, 'desk list 390');
  assert.deepEqual(failures, []);
});

// §2.2: the plans' home lists the releases still to go out, the soonest first.
test('chromium 375 px, dark: the plans\' home lists the releases to come', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 375, scheme: 'dark', path: '/plans', modulesOn: ['PLANNING'] });
  t.after(close);
  // *For you* (scene homeplans): the releases to come as a card, the soonest first.
  const upcoming = page.locator('[data-plan-card="releases"]');
  await upcoming.waitFor({ timeout: 10_000 });
  assert.deepEqual(await upcoming.locator('.card-rows a').allTextContents(), ['Autumn fixes', RELEASES[0].name]);
  assert.equal(await upcoming.getByRole('link').first().getAttribute('href'), `/hubs/${HUB.id}/releases`);
  // Each names what it waits for: the one entry still open, else done of all.
  await page.waitForFunction(() => [...document.querySelectorAll('[data-release-line]')].map((each) => each.textContent).join('|') === 'Entry 2 still open|4 of 10', null, { timeout: 10_000 });
  await noSidewaysScroll(page, 'plans 375 dark');
  assert.deepEqual(failures, []);
});

// §2.2, §7.9: the plans' home lists the sprints running in the hubs' collections, with where and
// how long, and a hub's row names its running sprint.
test('chromium 1280 px: the plans\' home lists the sprints running and says each hub\'s', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: '/plans', modulesOn: ['PLANNING'] });
  t.after(close);
  // *For you* (scene homeplans): the sprint that ends first as a card - its days left, what is done
  // of its scope, its burndown and where it runs.
  const card = page.locator('[data-plan-card="sprint"]');
  await card.locator('[data-burndown]').waitFor({ timeout: 10_000 });
  assert.match(await card.innerText(), /Running: Sprint Kranich/u);
  assert.match(await card.innerText(), /(Its last day has passed|\d+ days? left)/u);
  assert.match(await card.innerText(), /9 of 21 points done/u);
  assert.match(await card.innerText(), new RegExp(`${HUB.name} › ${SCRUM.name}`));
  await card.getByRole('button', { name: 'Open' }).click();
  await page.waitForFunction((id) => location.pathname.startsWith(`/collections/${id}`), SCRUM.id, { timeout: 10_000 });
  // The sprints' own tab lists them whole.
  await page.goto(`${served.origin}/plans?tab=sprints`);
  const running = page.getByRole('region', { name: 'Sprints running' });
  await running.waitFor({ timeout: 10_000 });
  assert.deepEqual(await running.locator('.name').allTextContents(), ['Sprint Kranich']);
  assert.deepEqual(failures, []);
});

// §7.9: a hub's overview says one line per product on there - the running sprint and the next
// release, the pages changed this week, the inventory - each leading to where it is about.
test('chromium 1280 px: a hub\'s overview says one line per product on there', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: `/hubs/${HUB.id}`, modulesOn: ['PLANNING', 'KNOWLEDGE', 'ASSETS', 'WHITEBOARD'] });
  t.after(close);
  // One boxed line per tool, then one for the content tree (the navigation concept §2.4): what it
  // says first, the tool it leads to at its end.
  const lines = page.getByRole('list', { name: 'What the products say here' });
  for (const id of ['plan', 'assets', 'contents']) await lines.locator(`[data-product-line="${id}"]`).waitFor({ timeout: 10_000 });
  // UC-WRK-36 (E-6): the hub's tools as tiles - the tool, one figure with its word, a line.
  assert.deepEqual(await lines.locator('[data-product-line]').evaluateAll((rows) => rows.map((row) => row.getAttribute('data-product-line'))), ['plan', 'assets', 'contents']);
  assert.match(await lines.locator('[data-product-line="plan"]').innerText(), /^Plan\s+\d+ days?\s+to the sprint's end\s+Sprint Kranich\s+Autumn fixes on [A-Z][a-z]{2} \d+$/);
  assert.equal(await lines.locator('[data-product-line="plan"] a').getAttribute('href'), `/hubs/${HUB.id}/plan`);
  // Every page and board of the fixture changed before this week began, the template included.
  assert.match(await lines.locator('[data-product-line="contents"]').innerText(), /^All contents\s+0\s+changed this week\s+No page and no whiteboard changed this week$/);
  assert.equal(await lines.locator('[data-product-line="contents"] a').getAttribute('href'), `/hubs/${HUB.id}/contents`);
  assert.match(await lines.locator('[data-product-line="assets"]').innerText(), /^Inventory\s+42\s+items\s+2 need attention$/);
  assert.equal(await lines.locator('[data-product-line="assets"] a').getAttribute('href'), `/hubs/${HUB.id}/inventory`);
  await noSidewaysScroll(page, 'hub overview');
  assert.deepEqual(failures, []);
});

// §7.9: the desk's, time's and goals' lines, and the hub's recent activity - the entries of its
// collections changed last - each leading to where it is about.
test('chromium 1280 px: a hub\'s overview says the desk, time and goals and what changed last', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: `/hubs/${HUB.id}`, modulesOn: ['WORKFLOWS', 'SERVICE_DESK', 'TIME', 'GOALS', 'WHITEBOARD'] });
  t.after(close);
  const lines = page.getByRole('list', { name: 'What the products say here' });
  for (const id of ['desk', 'time', 'goals', 'contents']) await lines.locator(`[data-product-line="${id}"]`).waitFor({ timeout: 10_000 });
  assert.deepEqual(await lines.locator('[data-product-line]').evaluateAll((rows) => rows.map((row) => row.getAttribute('data-product-line'))), ['desk', 'time', 'goals', 'contents']);
  // Both boards of the fixture changed before this week began.
  assert.match(await lines.locator('[data-product-line="contents"]').innerText(), /^All contents\s+0\s+changed this week\s+No page and no whiteboard changed this week$/);
  assert.match(await lines.locator('[data-product-line="desk"]').innerText(), /^Queues\s+3\s+open$/);
  assert.equal(await lines.locator('[data-product-line="desk"] a').getAttribute('href'), `/hubs/${HUB.id}/desk`);
  assert.match(await lines.locator('[data-product-line="time"]').innerText(), /^Time\s+6:30\s+this week/);
  assert.equal(await lines.locator('[data-product-line="time"] .day').count(), 7, 'the week as seven small bars');
  assert.equal(await lines.locator('[data-product-line="time"] a').getAttribute('href'), `/hubs/${HUB.id}/time`);
  assert.match(await lines.locator('[data-product-line="goals"]').innerText(), /^Goals\s+1\s+pursued/);
  assert.equal(await lines.locator('[data-product-line="goals"] a').getAttribute('href'), `/hubs/${HUB.id}/goals`);
  // What happened, in sentences: the person, what they did, and the thing with its tile, newest first;
  // the boards' changes among the entries' (work-surface §6.1, scene hub).
  const recent = page.getByRole('region', { name: 'What happened' });
  await recent.locator('[data-recent-entry]').first().waitFor({ timeout: 10_000 });
  await recent.getByText(OTHER.display_name).first().waitFor({ timeout: 10_000 });
  assert.deepEqual(await recent.locator('.title').allTextContents(), [...RECENT.map((entry) => entry.title), BOARDS[0].title, BOARDS[1].title]);
  assert.equal(await recent.locator('li').getByRole('link').first().getAttribute('href'), `/items/${RECENT[0].id}`);
  assert.match(await recent.locator('li').first().locator('.said').innerText(), new RegExp(`^${OTHER.display_name} created\\s*${RECENT[0].title}$`));
  // A step is said with the status it set, not as a move (work-surface §6.1).
  assert.match(await recent.locator('li').nth(1).locator('.said').innerText(), new RegExp(`^${OTHER.display_name} set\\s*${RECENT[1].title}\\s*to In progress$`));
  // *All* is the search over the hub, the latest change first.
  const all = new URL(await recent.locator('[data-recent-all]').getAttribute('href'), 'http://x');
  assert.equal(all.pathname, '/search');
  assert.equal(all.searchParams.get('f'), `hub:"${HUB.name}" sort:-updated`);
  assert.match(await recent.locator('[data-recent="board"] .said').first().innerText(), /changed the whiteboard/);
  const asked = queries.find((body) => body?.sort?.[0]?.field === 'updated_at');
  assert.deepEqual(asked.scope, { container_id: HUB.id, include_descendants: true });
  await noSidewaysScroll(page, 'hub overview, desk, time and goals');
  assert.deepEqual(failures, []);
});

// §5.1/§5.2: the plans' head carries its primary create act, as every product home does: *Plan a
// sprint* leads to a collection's sprints, the other collections in its menu.
test('chromium 1280 px: the plans\' home leads its create act to a collection\'s sprints', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: '/plans', modulesOn: ['PLANNING'] });
  t.after(close);
  const plan = page.getByRole('button', { name: 'Plan a sprint' });
  await plan.waitFor({ timeout: 10_000 });
  // The plans' band (E-1): its tile and figures - the sprints running, the releases due, the plans late.
  await page.waitForFunction(() => [...document.querySelectorAll('.band .figure')].map((each) => each.textContent?.trim()).join('|') === '1 sprint running|2 releases due|1 plan late', null, { timeout: 10_000 });
  await page.getByRole('button', { name: 'In another collection' }).click();
  await page.getByRole('menuitem', { name: `${HUB.name} › ${SCRUM.name}` }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');
  await plan.click();
  // A collection's sprints are its *Sprint* view (the navigation concept §8).
  await page.waitForURL(`**/collections/${DESK.id}/sprint?new=1`, { timeout: 5_000 });
  assert.deepEqual(failures, []);
});

// §6.11: one search for every kind: pages and boards beside entries, each in its own hit row, the
// kinds as facets with counts; with no entry matching, the screen does not say nothing matches.
for (const [width, scheme] of [[1280, 'dark'], [375, 'light']]) {
  test(`chromium ${width} px, ${scheme}: the search finds pages and boards beside entries, a facet per kind`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures, close } = await open(browser, { width, scheme, path: '/search', modulesOn: ['KNOWLEDGE', 'WHITEBOARD'] });
    t.after(close);
    // A phone's line waits in its filter sheet; wider, it stands over the hits.
    if (width < 600) {
      await page.getByRole('button', { name: 'Change filters' }).click();
      const sheet = page.getByRole('dialog', { name: 'Filters' });
      await sheet.getByRole('combobox').fill('release');
      await sheet.getByRole('button', { name: /^Show \d+ hits?$/u }).click();
    } else {
      await page.locator(LINE).fill('release');
    }
    const pageHit = page.locator('main a.hit[data-kind="PAGE"]');
    await pageHit.waitFor({ timeout: 10_000 });
    assert.equal(await pageHit.getAttribute('href'), `/pages/${PAGES[0].id}`);
    const boardHit = page.locator('main a.hit[data-kind="WHITEBOARD"]');
    assert.equal(await boardHit.getAttribute('href'), `/whiteboards/${BOARDS[0].id}`);
    assert.equal(await page.getByText('Nothing matches those words.').count(), 0);
    // A kind that found nothing is no facet; *all* counts what the kinds found.
    const facets = page.locator('main [data-facet]:visible');
    assert.deepEqual(await facets.evaluateAll((all) => all.map((facet) => facet.dataset.facet)), ['ALL', 'PAGE', 'WHITEBOARD']);
    await page.locator('main [data-facet="WHITEBOARD"]:visible').click();
    await pageHit.waitFor({ state: 'detached', timeout: 5_000 });
    assert.equal(await boardHit.count(), 1);
    await noSidewaysScroll(page, `search ${width} ${scheme}`);
    assert.deepEqual(failures, []);
  });
}

// Motion (user-experience.md §6): a home's rows arrive with the entrance role, staggered, by
// opacity and transform only - and nothing moves where the system or the product asks for less.
// Counted at `Element.animate`, because an arrival is over before a poll could see it.
test('chromium 1280 px: a home\'s rows arrive, and nothing moves under reduced motion', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const watch = () => {
    window.__arrivals = [];
    const animate = Element.prototype.animate;
    Element.prototype.animate = function (keyframes, options) {
      if (this.classList.contains('row')) window.__arrivals.push({ properties: Object.keys(keyframes[0] ?? {}).sort(), delay: options?.delay ?? 0, duration: options?.duration ?? 0 });
      return animate.call(this, keyframes, options);
    };
  };

  const first = await (async () => {
    const opened = await open(browser, { width: 1280, path: '/pages', modulesOn: ['KNOWLEDGE'] });
    t.after(opened.close);
    await opened.page.addInitScript(watch);
    await opened.page.reload();
    await opened.page.getByRole('region', { name: 'Waiting for your review' }).getByRole('link').first().waitFor({ timeout: 10_000 });
    const seen = await opened.page.evaluate(() => window.__arrivals ?? []);
    assert.deepEqual(opened.failures, []);
    return seen;
  })();
  assert.ok(first.length >= 1, `no row arrived: ${JSON.stringify(first)}`);
  for (const arrival of first) {
    assert.deepEqual(arrival.properties, ['opacity', 'transform'], 'an arrival moves only opacity and transform (rule 6)');
    assert.ok(arrival.duration > 0, 'the entrance role has no duration');
  }
  assert.equal(first[0].delay, 0, 'the first row waits');

  for (const [label, options, choice] of [['the system', { reducedMotion: 'reduce' }, null], ['the product', {}, 'reduced']]) {
    const opened = await open(browser, { width: 1280, path: '/pages', modulesOn: ['KNOWLEDGE'], ...options });
    t.after(opened.close);
    await opened.page.addInitScript(watch);
    if (choice) await opened.page.addInitScript((value) => localStorage.setItem('hubtask.motion', value), choice);
    await opened.page.reload();
    await opened.page.getByRole('region', { name: 'Waiting for your review' }).getByRole('link').first().waitFor({ timeout: 10_000 });
    assert.deepEqual(await opened.page.evaluate(() => window.__arrivals ?? []), [], `a row moved although ${label} asked for reduced motion`);
    assert.deepEqual(opened.failures, []);
  }
});

// §6.11, UC-WRK-18 check 9: goals, configuration items and dashboards are found by the search page
// and by the header search, from the one search over every kind, each hit opening where it lives.
test('chromium 1280 px: the search and the palette find goals, configuration items and dashboards', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: '/search', modulesOn: ['WHITEBOARD', 'GOALS', 'ASSETS'] });
  t.after(close);
  for (const [words, kind, href] of [
    ['kundinnen', 'GOAL', `/goals/${GOAL_ROWS[0].id}`],
    ['fortigate', 'ASSET', `/cis/${CIS[0].id}`],
    ['morgen', 'DASHBOARD', `/dashboards/${DASHBOARDS[0].id}`],
  ]) {
    await page.locator(LINE).fill(words);
    const hit = page.locator(`main a.hit[data-kind="${kind}"]`);
    await hit.waitFor({ timeout: 10_000 });
    assert.equal(await hit.getAttribute('href'), href, kind);
    assert.equal(await page.locator(`main [data-facet="${kind}"]`).count(), 1, `no facet for ${kind}`);
  }

  const palette = page.getByRole('combobox', { name: 'Search everything', exact: true });
  await palette.click();
  await palette.fill('kick-off');
  await page.locator('#search-menu').getByRole('option', { name: /Kick-off/u }).waitFor({ timeout: 10_000 });
  // The bar asks the kinds the page asks: inventory, goals and dashboards too.
  for (const [words, title] of [['fortigate', /FortiGate 60F/u], ['kundinnen', /Mehr Kundinnen gewinnen/u], ['morgen', /Betrieb am Morgen/u]]) {
    await palette.fill(words);
    await page.locator('#search-menu').getByRole('option', { name: title }).waitFor({ timeout: 10_000 });
  }
  assert.deepEqual(failures, []);
});

// UC-WRK-18 check 9: portfolios, releases and sprints are found by both searches, each with its
// facet, saying its kind, and opening where its product draws it.
test('chromium 1280 px: the search and the palette find portfolios, releases and sprints', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: '/search', modulesOn: ['GOALS', 'PLANNING'] });
  t.after(close);
  const cases = [
    ['programme', 'PORTFOLIO', '/portfolios/01a0e2e0-0000-7000-8000-000000000a01', 'Portfolio'],
    ['fixes', 'RELEASE', `/hubs/${HUB.id}/plan/releases`, 'Release'],
    ['sprint 24', 'SPRINT', `/collections/${SCRUM.id}/sprint`, 'Sprint'],
  ];
  for (const [words, kind, href, said] of cases) {
    await page.locator(LINE).fill(words);
    const hit = page.locator(`main a.hit[data-kind="${kind}"]`);
    await hit.waitFor({ timeout: 10_000 });
    assert.equal(await hit.getAttribute('href'), href, kind);
    assert.match(await hit.locator('.meta').innerText(), new RegExp(said, 'u'), kind);
    assert.equal(await page.locator(`main [data-facet="${kind}"]`).count(), 1, `no facet for ${kind}`);
  }
  const palette = page.getByRole('combobox', { name: 'Search everything', exact: true });
  for (const [words, title] of [['programme', /Autumn programme/u], ['fixes', /Autumn fixes/u], ['sprint 24', /Sprint 24/u]]) {
    await palette.click();
    await palette.fill(words);
    await page.locator('#search-menu').getByRole('option', { name: title }).waitFor({ timeout: 10_000 });
    await page.keyboard.press('Escape');
  }
  assert.deepEqual(failures, []);
});

// §2.2: a home's hub rows say each hub's state, not its name alone - the plans' its next release,
// the assets' its inventory - with the hub's own figure where it counts one.
test('chromium 1280 px: the plans\' and the assets\' hub rows say each hub\'s state', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: '/plans', modulesOn: ['PLANNING', 'ASSETS'] });
  t.after(close);
  // The plans in the reader's hubs: the phase now, the sprint and the release, the delay in its
  // words, and the phases as one bar (scene homeplans).
  const hubs = page.getByRole('region', { name: 'Plans in your hubs' });
  const row = hubs.locator('li').filter({ hasText: HUB.name });
  await row.locator('[data-plan-late]').waitFor({ timeout: 10_000 });
  assert.equal(await row.locator('[data-hub-line]').innerText(), 'Shell · 4 days late');
  assert.deepEqual(await row.locator('[data-phase]').evaluateAll((all) => all.map((each) => each.getAttribute('data-phase'))), ['done', 'now']);
  assert.equal(await row.locator('[data-phases]').getAttribute('aria-label'), 'Phases: Groundwork, Shell');

  await page.goto(`${served.origin}/assets`);
  const assets = page.getByRole('region', { name: 'In your hubs' }).locator('li').filter({ hasText: HUB.name });
  await assets.locator('[data-hub-line]').waitFor({ timeout: 10_000 });
  assert.match(await assets.locator('[data-hub-line]').innerText(), /42 configuration items · 2 need attention/u);
  // What needs attention across the hubs, each with its hub (scene homeassets).
  const attention = page.locator('[data-attention] [data-attention-ci]');
  await attention.first().waitFor({ timeout: 10_000 });
  assert.equal(await attention.count(), 2);
  assert.match(await attention.first().innerText(), new RegExp(HUB.name));
  assert.deepEqual(failures, []);
});
