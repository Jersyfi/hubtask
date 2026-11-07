// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The navigation concept's workspace, served from stubs: three hubs and eleven more, their
// collections, contents, queues, sprints, goals and releases, as the mockup draws them. Shared by
// the sidebar's walk (`kompass.test.mjs`) and the screens' walk (`kompassscreens.test.mjs`).

import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { PERMISSIONS, PRESETS, ROLES, fallback } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

export const id = (n) => `01a0e2e0-0000-7000-8000-${String(n).padStart(12, '0')}`;
export const ACCOUNT = { id: id(1), kind: 'USER', display_name: 'Mara Lind', email: 'mara@example.invalid', status: 'ACTIVE', locale: 'en', onboarding_completed_at: '2026-09-01T00:00:00Z', products_seen: ['PLANNING', 'KNOWLEDGE', 'WHITEBOARD', 'SERVICE_DESK', 'ASSETS', 'TIME', 'GOALS', 'AGENTS'] };
export const hub = (n, name, color) => ({ id: id(n), type: 'HUB', parent_id: null, name, color_token: color, order_key: `a${n}`, version: 1 });
export const IT = hub(100, 'IT operations', 'teal');
export const PD = hub(101, 'Product development', 'orange');
export const HOME = hub(102, 'Household', 'violet');
export const OTHERS = ['Marketing', 'People', 'Finance', 'Facilities', 'Sales', 'Purchasing', 'Legal', 'Research', 'Support', 'Design', 'Data'].map((name, i) => hub(110 + i, name, 'blue'));
export const HUBS = [PD, IT, HOME, ...OTHERS];
export const collection = (n, parent, name, extra = {}) => ({ id: id(n), type: 'COLLECTION', parent_id: parent.id, name, order_key: `a${n}`, version: 1, ...extra });
export const DESK = collection(200, IT, 'Service desk', { structure: { settings: { entry_types: { TASK: ['incident'] } } }, figure: { kind: 'DUE', due: 1 } });
export const CHANGES = collection(201, IT, 'Changes', { figure: { kind: 'OPEN', open: 4 } });
export const WEB = collection(210, PD, 'Web app', { figure: { kind: 'OPEN', open: 18 } });
export const MOBILE = collection(212, PD, 'Mobile', { figure: { kind: 'OPEN', open: 12 } });
export const DESIGN = collection(213, PD, 'Design', { figure: { kind: 'OPEN', open: 14 } });
export const BUGS = collection(211, PD, 'Bugs', { figure: { kind: 'DUE', due: 3 } });
export const TODO = collection(220, HOME, 'To do');
export const COLLECTIONS = [DESK, CHANGES, WEB, MOBILE, DESIGN, BUGS, TODO];
/** A day counted from today, as a calendar date. */
export const day = (offset) => new Date(Date.now() + offset * 86_400_000).toISOString().slice(0, 10);
export const sprint = (n, at, name, endIn) => ({ id: id(n), collection_id: at.id, name, goal: '', start_on: day(endIn - 13), end_on: day(endIn), state: 'ACTIVE', unit: 'POINTS', committed_value: 0, committed_count: 29, completed_value: 0, completed_count: 18, started_at: null, closed_at: null, version: 1 });
export const SPRINTS = { [WEB.id]: [sprint(600, WEB, 'Sprint 42', 5)], [MOBILE.id]: [sprint(601, MOBILE, 'Sprint 9', 8)] };
// Product development's handbook: pages nested two deep, boards among them and one not yet filed.
export const pdPage = (n, title, parent = null) => ({ id: id(n), hub_id: PD.id, parent_id: parent, title, order_key: `a${n}`, template: false, version: 1 });
export const PD_PAGES = [
  pdPage(310, 'Product handbook'), pdPage(311, 'Vision and principles', id(310)), pdPage(312, 'How we plan', id(310)),
  pdPage(313, 'Release notes'), pdPage(314, 'Release 3.1', id(313)), pdPage(315, 'Release 3.2', id(313)),
  pdPage(316, 'Architecture decisions'), pdPage(317, 'Onboarding'), pdPage(318, 'Meeting notes'),
];
export const PD_BOARDS = [
  { id: id(410), hub_id: PD.id, title: 'Roadmap sketch', parent_page_id: id(312), order_key: 'a1', version: 1 },
  { id: id(411), hub_id: PD.id, title: 'Retro board', parent_page_id: null, order_key: null, version: 1 },
];
export const GOALS = ['Ship the new editor', 'Halve the bug backlog', 'Faster first load', 'Mobile parity', 'Accessible everywhere']
  .map((title, i) => ({ id: id(700 + i), hub_id: PD.id, period_id: id(799), parent_id: null, title, status: i === 2 ? 'AT_RISK' : 'ON_TRACK' }));
export const RELEASES = [{ id: id(710), hub_id: PD.id, name: 'Release 3.2', state: 'PLANNED', release_on: day(14), version: 1 }];
/** What is on the reader today: five entries due by tonight, and one later. */
export const MINE = [0, 0, 0, -1, -2, 6].map((offset, i) => ({ id: id(800 + i), title: `Entry ${i + 1}`, collection_id: WEB.id, due_at: `${day(offset)}T00:00:00Z`, due_date_only: true, completion: { is_completed: false } }));
export const ARRIVALS = [0, 1, 2].map((i) => ({ id: id(900 + i), channel: 'EMAIL', attachments: [], status: 'NEW', received_at: '2026-11-07T08:00:00Z' }));
export const APPROVALS = [0, 1].map((i) => ({ id: id(950 + i), state: 'PENDING' }));
export const KB = { id: id(300), hub_id: IT.id, parent_id: null, title: 'Knowledge base', order_key: 'a1', template: false, version: 1 };
export const VPN = { id: id(301), hub_id: IT.id, parent_id: KB.id, title: 'Set up the VPN', order_key: 'a1', template: false, version: 1 };
export const RUNBOOKS = { id: id(302), hub_id: IT.id, parent_id: null, title: 'Runbooks', order_key: 'a2', template: false, version: 1 };
export const TEMPLATE = { id: id(303), hub_id: IT.id, parent_id: null, title: 'Incident review template', order_key: 'a3', template: true, version: 1 };
export const MAP = { id: id(400), hub_id: IT.id, title: 'Network map', parent_page_id: null, order_key: 'a5', version: 1 };
export const QUEUES = [
  { view_id: id(500), key: 'about_to_breach', name: 'About to breach', count: 1, at_risk: 1 },
  { view_id: id(501), key: 'mine', name: 'Mine', count: 3, at_risk: 0 },
  { view_id: id(502), key: 'unassigned', name: 'Unassigned', count: 2, at_risk: 0 },
];
export const PAGE = { items: [], page: { next_cursor: null, has_more: false } };

/** `extra(route, request, url)` answers first where a walk needs more; it returns whether it did. */
export function stubFor({ modulesOn, hubModules = {}, stars = [], prefs = {}, administration = true, owner = false, extra }) {
  const written = { preferences: [], stars: [] };
  const handler = async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    if (extra && (await extra(route, request, url))) return;
    // `owner`: the reader owns the workspace, so every ＋ and ⋯ a hub offers is drawn.
    if (owner && path.endsWith('/api/v1/memberships')) {
      const scope = url.searchParams.get('scope_type');
      const rows = scope === 'TENANT' || scope === 'HUB' ? [{ id: `m-${scope}`, scope_type: scope, scope_id: url.searchParams.get('scope_id'), account_id: ACCOUNT.id, role: 'OWNER' }] : [];
      return route.fulfill({ json: { ...PAGE, data: rows } });
    }
    if (path.endsWith('/api/v1/stream')) return route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'retry: 3600000\n\n' });
    if (path.endsWith('/api/v1/sync:snapshot')) {
      const records = [...HUBS, ...COLLECTIONS].map((each) => JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: each.id, container_id: each.parent_id, payload: each }));
      return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${records.join('\n')}\n{"cursor":"c-e2e"}\n` });
    }
    if (path.endsWith('/api/v1/sync:pull')) return route.fulfill({ json: { changes: [], cursor: 'c-e2e', has_more: false, tombstone_window_days: 90 } });
    if (path.endsWith(`/api/v1/accounts/${ACCOUNT.id}/preferences`) && request.method() === 'PATCH') {
      const body = request.postDataJSON();
      written.preferences.push(body);
      Object.assign(prefs, body);
      return route.fulfill({ json: { ...ACCOUNT, ...prefs } });
    }
    if (path.endsWith('/api/v1/accounts/me')) return route.fulfill({ json: { ...ACCOUNT, ...prefs } });
    if (path.endsWith(`/api/v1/accounts/${ACCOUNT.id}/stars`)) {
      if (request.method() === 'PUT') {
        const body = request.postDataJSON();
        written.stars.push(body);
        stars.splice(0, stars.length, ...body.stars);
      }
      return route.fulfill({ json: { stars, version: written.stars.length } });
    }
    if (path.endsWith('/api/v1/meta/capabilities')) {
      return route.fulfill({ json: { product_version: '0.9.0', api_version: 'v1', tenancy_mode: 'single', item_types: [], view_layouts: ['LIST_COLLAPSED', 'LIST_EXPANDED', 'KANBAN', 'TIMELINE'], supported_locales: [{ locale: 'en', direction: 'ltr' }], workspace_permissions: administration ? PERMISSIONS.OWNER : ['READ', 'WRITE_ITEMS'], modules_on: modulesOn, ...(owner ? { roles: ROLES } : {}) } });
    }
    const modules = /\/api\/v1\/containers\/([^/]+)\/modules$/.exec(path);
    if (modules) {
      const at = COLLECTIONS.find((each) => each.id === modules[1])?.parent_id ?? modules[1];
      return route.fulfill({ json: { modules_on: hubModules[at] ?? modulesOn } });
    }
    // The contract's array; the general stub's empty page would break the new-collection dialog.
    if (path.endsWith('/api/v1/presets')) return route.fulfill({ json: PRESETS });
    if (path.endsWith('/api/v1/entry-types')) {
      return route.fulfill({ json: { data: [{ key: 'incident', name: 'Incident', level: 'TASK', hub_id: null, form: { channels: [{ channel: 'PORTAL' }] } }], page: PAGE.page } });
    }
    if (path.endsWith(`/api/v1/hubs/${IT.id}/queues`)) return route.fulfill({ json: { data: QUEUES } });
    // A queue is a saved view of its hub; a starred one is read by its own address.
    const view = /\/api\/v1\/views\/([^/:]+)$/.exec(path);
    const queue = view && QUEUES.find((each) => each.view_id === view[1]);
    if (queue) return route.fulfill({ json: { id: queue.view_id, key: queue.key, name: queue.name, scope_type: 'HUB', scope_id: IT.id, queue: true, version: 1 } });
    if (/\/api\/v1\/hubs\/[^/]+\/queues$/.test(path)) return route.fulfill({ json: { data: [] } });
    if (path.endsWith(`/api/v1/hubs/${IT.id}/pages`) && !url.searchParams.has('templates')) return route.fulfill({ json: { data: [KB, VPN, RUNBOOKS, TEMPLATE], ...PAGE } });
    if (path.endsWith(`/api/v1/hubs/${IT.id}/whiteboards`)) return route.fulfill({ json: { data: [MAP], ...PAGE } });
    if (path.endsWith(`/api/v1/hubs/${PD.id}/pages`) && !url.searchParams.has('templates')) return route.fulfill({ json: { data: PD_PAGES, ...PAGE } });
    if (path.endsWith(`/api/v1/hubs/${PD.id}/whiteboards`)) return route.fulfill({ json: { data: PD_BOARDS, ...PAGE } });
    if (path.endsWith(`/api/v1/hubs/${PD.id}/releases`)) return route.fulfill({ json: { data: RELEASES, ...PAGE } });
    if (path.endsWith(`/api/v1/hubs/${IT.id}/cmdb-health`)) return route.fulfill({ json: { total: 120, problems: Array.from({ length: 6 }, (_, i) => ({ ci_id: id(980 + i), kind: 'STALE' })) } });
    if (path.endsWith('/api/v1/goals') && url.searchParams.get('hub_id') === PD.id) return route.fulfill({ json: { data: GOALS, ...PAGE } });
    // The reader's week and timer, empty: *Today* draws its time line from them.
    if (/\/api\/v1\/timesheets\/[^/]+$/.test(path)) return route.fulfill({ json: { week: url.pathname.split('/').pop(), days: [], minutes: 0, entries: [], state: 'OPEN' } });
    if (path.endsWith('/api/v1/timer')) return route.fulfill({ json: { running: null } });
    if (path.endsWith('/api/v1/jumble/entries')) return route.fulfill({ json: { data: url.searchParams.get('status') === 'NEW' ? ARRIVALS : [], page: { next_cursor: null } } });
    if (path.endsWith('/api/v1/approvals')) return route.fulfill({ json: { data: APPROVALS, page: { next_cursor: null, has_more: false } } });
    if (path.endsWith('/api/v1/search') && (request.postDataJSON()?.filter?.nodes ?? []).some((node) => node.field === 'assignee_id')) {
      return route.fulfill({ json: { data: MINE, page: { next_cursor: null, has_more: false } } });
    }
    if (path.endsWith('/api/v1/items:query')) {
      const body = request.postDataJSON() ?? {};
      const nodes = body.filter?.nodes ?? [];
      if (nodes.some((node) => node.field === 'collection_id' && node.value?.includes?.(DESK.id))) return route.fulfill({ json: { data: [], total: 1, page: { next_cursor: null, has_more: false } } });
      return route.fulfill({ json: { data: [], total: 0, page: { next_cursor: null, has_more: false } } });
    }
    if (/\/api\/v1\/collections\/[^/]+\/sprints$/.test(path)) {
      const at = /collections\/([^/]+)\/sprints/.exec(path)[1];
      return route.fulfill({ json: { data: SPRINTS[at] ?? [], runs_sprints: at in SPRINTS } });
    }
    // The sprint and the backlog views: one entry waiting in the backlog, the sprint without a burndown yet.
    if (/\/api\/v1\/collections\/[^/]+\/sprint-backlog$/.test(path)) return route.fulfill({ json: { data: [{ item_id: id(650), title: 'Offline mode', estimate: null }] } });
    if (/\/api\/v1\/collections\/[^/]+\/velocity$/.test(path)) return route.fulfill({ json: { unit: 'POINTS', average: 0, sprints: [] } });
    const running = Object.values(SPRINTS).flat().find((each) => path.endsWith(`/api/v1/sprints/${each.id}`));
    if (running) return route.fulfill({ json: { sprint: running, entries: [], burndown: [] } });
    if (path.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: HUBS } });
    if (path.endsWith('/api/v1/containers')) return route.fulfill({ json: { ...PAGE, data: COLLECTIONS.filter((each) => each.parent_id === url.searchParams.get('parent_id')) } });
    // A plan's schedule, empty: the tool's and a collection's timeline open on it.
    if (/\/api\/v1\/plans\/[^/]+\/schedule$/.test(path)) return route.fulfill({ json: { zone: 'UTC', finish: null, entries: [], dependencies: [], conflicts: [], unscheduled: [], undated: [], critical_path: [] } });
    if (/\/api\/v1\/plans\/[^/]+\/baselines$/.test(path)) return route.fulfill({ json: { data: [] } });
    if (/\/api\/v1\/plans\/[^/]+\/tolerance$/.test(path)) return route.fulfill({ json: { time_tolerance_days: null, source: 'DEFAULT', locked_by: null, baseline_id: null, exception: null } });
    if (/\/(labels|buckets|views|templates|custom-fields|policies|members)$/.test(path)) return route.fulfill({ json: [] });
    return fallback(route, request, path);
  };
  return { handler, written };
}

export const served = await serve(DIST);

/** `ready` says when the page is up: the avatar by default; a phone's deeper step has the bar's ‹ instead. */
export async function open(browser, { width = 1440, height = 900, path = '/', storage = {}, hasTouch = false, ready, ...options }) {
  const context = await browser.newContext({ viewport: { width, height }, hasTouch });
  const { handler, written } = stubFor(options);
  await context.route('**/api/v1/**', handler);
  await context.addInitScript((kept) => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    for (const [key, value] of Object.entries(kept)) localStorage.setItem(key, value);
  }, storage);
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}${path}`);
  await (ready ? ready(page) : page.getByRole('button', { name: ACCOUNT.display_name })).waitFor({ timeout: 15_000 });
  return { page, failures, written, close: () => context.close() };
}

export const ALL = ['WORKFLOWS', 'PLANNING', 'KNOWLEDGE', 'WHITEBOARD', 'SERVICE_DESK', 'ASSETS', 'TIME', 'GOALS', 'AGENTS'];
export const side = (page) => page.locator('aside.sidenav');
export const texts = async (locator) => (await locator.allInnerTexts()).map((each) => each.split('\n')[0].trim()).filter(Boolean);
/** The section heads of the middle, in order. */
export const heads = (page) => texts(side(page).locator('section > :first-child'));
