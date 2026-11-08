// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The navigation concept's workspace, served from stubs: three hubs and eleven more, their
// collections, contents, queues, sprints, goals and releases, as the mockup draws them. Shared by
// the sidebar's walk (`kompass.test.mjs`) and the screens' walk (`kompassscreens.test.mjs`).

import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { MANIFEST, PERMISSIONS, PRESETS, ROLES, fallback, presetStructure } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

export const id = (n) => `01a0e2e0-0000-7000-8000-${String(n).padStart(12, '0')}`;
export const ACCOUNT = { id: id(1), kind: 'USER', display_name: 'Mara Lind', email: 'mara@example.invalid', status: 'ACTIVE', locale: 'en', onboarding_completed_at: '2026-09-01T00:00:00Z', products_seen: ['PLANNING', 'KNOWLEDGE', 'WHITEBOARD', 'SERVICE_DESK', 'ASSETS', 'TIME', 'GOALS', 'AGENTS'] };
export const hub = (n, name, color) => ({ id: id(n), type: 'HUB', parent_id: null, name, color_token: color, order_key: `a${n}`, version: 1, structure: presetStructure('PROJECT') });
export const IT = hub(100, 'IT operations', 'teal');
export const PD = hub(101, 'Product development', 'orange');
export const HOME = hub(102, 'Household', 'violet');
export const OTHERS = ['Marketing', 'People', 'Finance', 'Facilities', 'Sales', 'Purchasing', 'Legal', 'Research', 'Support', 'Design', 'Data'].map((name, i) => hub(110 + i, name, 'blue'));
export const HUBS = [PD, IT, HOME, ...OTHERS];
export const collection = (n, parent, name, extra = {}) => ({ id: id(n), type: 'COLLECTION', parent_id: parent.id, name, order_key: `a${n}`, version: 1, ...extra, structure: { ...presetStructure('PROJECT'), settings: { ...presetStructure('PROJECT').settings, ...extra.structure?.settings } } });
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
// Two approvals in the contract's shape: *Waiting for your yes* draws each as a card.
export const APPROVALS = ['Laptop for the new designer', 'Firewall rule for the build server'].map((title, i) => ({
  id: id(950 + i), item_id: id(960 + i), item_title: title, item_number: 70 + i, on_behalf_of: null, workflow: 'requests', step: 'approve',
  item_version: 1, state: 'PENDING', current_level: 1, levels: [{ rule: 'ANY', count: 1, approvers: [ACCOUNT.id], roles: [], needed: 1 }],
  decisions: [], requested_by: id(2), requested_at: '2026-11-06T09:00:00Z', decided_at: null, due_at: null, may_decide: true, version: 1,
}));
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
/** A hub's inventory health as `GET /hubs/{id}/cmdb-health` answers it, nothing wrong. */
const HEALTH = { total: 0, completeness: 1, correctness: 1, stale: 0, orphans: 0, duplicates: 0, without_owner: 0, review_overdue: 0, classes: [], problems: [], stale_relations: 0, services: [], sources: [] };
/** The one request a queue lists. */
export const QUEUE_REQUEST = { id: id(1200), title: 'Printer on the 2nd floor does not print', collection_id: DESK.id, number: 41, created_by: id(1), assignee_id: null, status: null, completion: { is_completed: false } };

// Each collection's entries and its board's columns, so a collection, its board and its sprint
// show work rather than an empty list. The desk keeps its own answer (one request, by the queue).
const COLUMNS = ['To do', 'Doing', 'Done'];
export const BUCKETS = Object.fromEntries(COLLECTIONS.map((at, c) => [at.id, COLUMNS.map((name, i) => ({
  id: id(3000 + c * 10 + i), collection_id: at.id, name, order_key: `a${i}`, wip_limit: null, is_done_bucket: i === 2, color_token: null, version: 1,
}))]));
const TITLES = {
  [WEB.id]: ['Sign-in with passkeys', 'Faster first load', 'Offline banner', 'Keyboard shortcuts sheet', 'Dark mode contrast', 'Export to CSV'],
  [MOBILE.id]: ['Push notifications', 'Swipe to complete', 'Share sheet', 'Widget for today', 'Haptics on drop', 'Crash on rotate'],
  [DESIGN.id]: ['Icon set review', 'Empty states', 'Colour tokens audit', 'Onboarding illustrations'],
  [BUGS.id]: ['Timeline jumps on scroll', 'Duplicate toast', 'Avatar blurry on retina'],
  [CHANGES.id]: ['Upgrade the database', 'Rotate the certificates', 'New firewall rule', 'Move the backups'],
  [TODO.id]: ['Pay the electricity bill', 'Book the chimney sweep', 'Buy a new kettle'],
};
export const ENTRIES = Object.entries(TITLES).flatMap(([at, titles], c) => titles.map((title, i) => ({
  id: id(4000 + c * 20 + i), type: 'TASK', title, collection_id: at, parent_id: null, depth: 0, order_key: `a${i}`, version: 1, number: 10 + i,
  completion: { is_completed: i % 4 === 3 }, labels: [], label_ids: [], bucket_id: BUCKETS[at][i % 4 === 3 ? 2 : i % 2].id,
  ...(i === 1 ? { due_at: `${day(3)}T09:00:00Z`, due_date_only: true } : {}),
})));
/** Every entry a link on the walks' screens opens: the collections', *Today*'s, the backlog's, the approvals'. */
const asEntry = (each, at = WEB.id) => ({ type: 'TASK', collection_id: at, parent_id: null, depth: 0, order_key: 'a', version: 1, number: 1, completion: { is_completed: false }, labels: [], label_ids: [], ...each });
const OPENABLE = () => [
  ...ENTRIES, ...MINE.map((each) => asEntry(each)), asEntry({ id: id(650), title: 'Offline mode' }, MOBILE.id),
  ...APPROVALS.map((each) => asEntry({ id: each.item_id, title: each.item_title, number: each.item_number }, DESK.id)),
];
/** The sprint's entries, as `GET /sprints/{id}` lists them: the collection's first four. */
const sprintEntries = (at) => ENTRIES.filter((each) => each.collection_id === at).slice(0, 4).map((each) => ({
  item_id: each.id, title: each.title, added_at: '2026-11-01T08:00:00Z', removed_at: null, committed: true, outcome: null,
  done: each.completion.is_completed, started: true, completed_at: null, estimate: 3,
}));
/** Every page of every hub a walk opens: the handbook and IT operations' knowledge. */
const PAGES = () => [...PD_PAGES, KB, VPN, RUNBOOKS, TEMPLATE];
const BOARDS = () => [...PD_BOARDS, MAP];
/** A page as `GET /pages/{id}` answers it (PageRead). */
export function pageRead(found, writable = true) {
  const parent = PAGES().find((each) => each.id === found.parent_id);
  return {
    icon: null, position: 0, content_language: 'en', owner_id: null, verified_until: null, verified_at: null, verified_by: null, due_for_review: false,
    archived: false, restricted: false, daily_for: null, trashed_at: null, created_by: ACCOUNT.id, created_at: '2026-11-01T08:00:00Z', updated_by: ACCOUNT.id,
    body: 'Welcome to the team.', labels: [], drafts: false, unpublished: false, draft: null, shared: false,
    ...found, writable, ancestors: parent ? [{ id: parent.id, title: parent.title }] : [], updated_at: '2026-11-06T10:00:00Z',
  };
}
/** A whiteboard as `GET /whiteboards/{id}` answers it (WhiteboardRead): two stickies on it. */
export function boardRead(found, writable = true) {
  const sticky = (n, x, text) => ({ id: `s${n}`, kind: 'STICKY', x, y: 80, width: 180, height: 120, text, color_token: null });
  const elements = [sticky(1, 80, 'What went well'), sticky(2, 320, 'What to change')];
  return {
    element_count: elements.length, elements, trashed_at: null, created_by: ACCOUNT.id, created_at: '2026-11-01T08:00:00Z', updated_by: ACCOUNT.id,
    updated_at: '2026-11-06T10:00:00Z', parent_page_id: null, order_key: null, ...found, writable,
  };
}

/** `extra(route, request, url)` answers first where a walk needs more; it returns whether it did. */
export function stubFor({ modulesOn, hubModules = {}, stars = [], prefs = {}, administration = true, owner = false, extra }) {
  const written = { preferences: [], stars: [], pages: [], boards: [] };
  const handler = async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    if (extra && (await extra(route, request, url))) return;
    // A queue's one request, and the move dialog's pre-check: IT operations with obstacles, every
    // other hub with warnings only.
    if (path.endsWith('/api/v1/items:query') && (request.postDataJSON()?.filter?.nodes ?? []).some((node) => node.field === 'sla_standing')) {
      return route.fulfill({ json: { data: [QUEUE_REQUEST], groups: [], total: 1, page: { next_cursor: null, has_more: false } } });
    }
    if (path.endsWith(':move-preview')) {
      const to = request.postDataJSON()?.target_parent_id;
      return route.fulfill({
        json: to === IT.id
          ? {
              movable: false,
              obstacles: [{ code: 'containers.name_taken', params: {} }, { code: 'entry_types.hub_bound', params: {} }],
              consequences: [
                { code: 'containers.move_loses_access', params: { count: '2', account_ids: `${id(61)},${id(62)}` } },
                { code: 'containers.move_takes_structure', params: { hub_id: IT.id } },
                { code: 'containers.move_keeps_releases', params: { count: '4', hub_id: PD.id } },
              ],
            }
          : { movable: true, obstacles: [], consequences: [{ code: 'containers.move_loses_access', params: { count: '1', account_ids: id(61) } }] },
      });
    }
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
      // The shared manifest's types and query fields: a board groups by `bucket_id` because the manifest
      // says it may, and a collection offers *Add an entry* for a type the manifest declares.
      return route.fulfill({ json: { product_version: '0.9.0', api_version: 'v1', tenancy_mode: 'single', item_types: MANIFEST.item_types, query_fields: MANIFEST.query_fields, view_layouts: ['LIST_COLLAPSED', 'LIST_EXPANDED', 'KANBAN', 'TIMELINE'], supported_locales: [{ locale: 'en', direction: 'ltr' }], workspace_permissions: administration ? PERMISSIONS.OWNER : ['READ', 'WRITE_ITEMS'], modules_on: modulesOn, ...(owner ? { roles: ROLES } : {}) } });
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
    // A server the health lists, opened from its line.
    const ci = /\/api\/v1\/cis\/([^/]+)$/.exec(path);
    if (ci && request.method() === 'GET' && Number(ci[1].slice(-3)) >= 980 && Number(ci[1].slice(-3)) < 986) {
      return route.fulfill({ json: {
        id: ci[1], hub_id: IT.id, class: 'server', name: `srv-app-0${ci[1].slice(-1)}`, stage: 'PRODUCTION', status: 'ACTIVE', attributes: {}, provenance: {},
        owner_account_id: null, owner_group_id: null, location_id: null, protection: { c: null, i: null, a: null, fixed: [], reason: '' }, effective_protection: { c: null, i: null, a: null },
        has_asset: false, review_due: null, quantity: 1, last_seen_at: null, merged_into: null, relations: [], created_by: ACCOUNT.id, created_at: '2026-10-01T08:00:00Z',
        updated_by: ACCOUNT.id, updated_at: '2026-10-01T08:00:00Z', version: 1,
      } });
    }
    if (path.endsWith(`/api/v1/hubs/${IT.id}/cmdb-health`)) return route.fulfill({ json: { ...HEALTH, total: 120, stale: 6, problems: Array.from({ length: 6 }, (_, i) => ({ ci_id: id(980 + i), class: 'server', kinds: ['STALE'] })) } });
    if (path.endsWith('/api/v1/goals') && url.searchParams.get('hub_id') === PD.id) return route.fulfill({ json: { data: GOALS, ...PAGE } });
    // The reader's week and timer, empty: *Today* draws its time line from them.
    if (/\/api\/v1\/timesheets\/[^/]+$/.test(path)) return route.fulfill({ json: { week: url.pathname.split('/').pop(), days: [], minutes: 0, entries: [], records: [], state: 'OPEN' } });
    if (path.endsWith('/api/v1/timer')) return route.fulfill({ json: { running: null } });
    if (path.endsWith('/api/v1/jumble/entries')) return route.fulfill({ json: { data: url.searchParams.get('status') === 'NEW' ? ARRIVALS : [], page: { next_cursor: null } } });
    if (path.endsWith('/api/v1/approvals')) return route.fulfill({ json: { data: APPROVALS, page: { next_cursor: null, has_more: false } } });
    if (path.endsWith('/api/v1/search') && (request.postDataJSON()?.filter?.nodes ?? []).some((node) => node.field === 'assignee_id')) {
      return route.fulfill({ json: { data: MINE, page: { next_cursor: null, has_more: false } } });
    }
    if (path.endsWith('/api/v1/items:query')) {
      const body = request.postDataJSON() ?? {};
      const nodes = body.filter?.nodes ?? [];
      const none = { next_cursor: null, has_more: false };
      if (nodes.some((node) => node.field === 'collection_id' && node.value?.includes?.(DESK.id))) return route.fulfill({ json: { data: [], groups: [], total: 1, page: none } });
      const at = body.scope?.container_id;
      const level = at && !body.scope?.item_id ? ENTRIES.filter((each) => each.collection_id === at) : [];
      if (body.group_by && BUCKETS[at]) {
        const groups = [...BUCKETS[at].map((each) => each.id), null].map((key) => {
          const data = level.filter((each) => (each.bucket_id ?? null) === key);
          return { key, count: data.length, data, page: none };
        });
        return route.fulfill({ json: { data: [], groups, total: level.length, page: none } });
      }
      return route.fulfill({ json: { data: level, groups: [], total: level.length, page: none } });
    }
    const entry = OPENABLE().find((each) => path.endsWith(`/api/v1/items/${each.id}`));
    if (entry && request.method() === 'GET') return route.fulfill({ json: entry });
    const buckets = /\/api\/v1\/containers\/([^/]+)\/buckets$/.exec(path);
    if (buckets && request.method() === 'GET') return route.fulfill({ json: BUCKETS[buckets[1]] ?? [] });
    // A page or a whiteboard opened, and one made: answered in the contract's shapes, so opening
    // content and *New page* or *New whiteboard* land on a page rather than on a list's shape.
    const opened = /\/api\/v1\/pages\/([^/:]+)$/.exec(path);
    const known = opened && PAGES().find((each) => each.id === opened[1]);
    if (known && request.method() === 'GET') return route.fulfill({ json: pageRead(known, owner) });
    const board = /\/api\/v1\/whiteboards\/([^/:]+)$/.exec(path);
    const drawn = board && BOARDS().find((each) => each.id === board[1]);
    if (drawn && request.method() === 'GET') return route.fulfill({ json: boardRead(drawn, owner) });
    if (path.endsWith('/api/v1/pages') && request.method() === 'POST') {
      const body = request.postDataJSON() ?? {};
      const made = { id: id(5000 + written.pages.length), hub_id: body.hub_id, parent_id: body.parent_id ?? null, title: body.title ?? '', order_key: 'z', template: false, version: 1 };
      written.pages.push(made);
      PD_PAGES.push(made);
      return route.fulfill({ status: 201, json: pageRead(made) });
    }
    // An opened page's own lists, a release, a plan's workload and the other hubs' inventory health,
    // in the contract's shapes rather than a list's.
    if (/\/api\/v1\/pages\/[^/]+\/labels$/.test(path)) return route.fulfill({ json: { labels: [] } });
    if (/\/api\/v1\/pages\/[^/]+\/[a-z-]+$/.test(path) && request.method() === 'GET') return route.fulfill({ json: { data: [], ...PAGE } });
    const release = /\/api\/v1\/releases\/([^/]+)$/.exec(path);
    const shipped = release && RELEASES.find((each) => each.id === release[1]);
    if (shipped && request.method() === 'GET') return route.fulfill({ json: { description: '', released_at: null, notes_page_id: null, ...shipped, entries: [], done_count: 0, open_count: 0 } });
    const workload = /\/api\/v1\/plans\/([^/]+)\/workload$/.exec(path);
    if (workload) return route.fulfill({ json: { plan_id: workload[1], aggregated: false, cells: [], team: [], unscheduled_minutes: 0 } });
    if (/\/api\/v1\/hubs\/[^/]+\/cmdb-health$/.test(path) && !path.includes(IT.id)) return route.fulfill({ json: HEALTH });
    if (path.endsWith('/api/v1/whiteboards') && request.method() === 'POST') {
      const body = request.postDataJSON() ?? {};
      const made = { id: id(5500 + written.boards.length), hub_id: body.hub_id, title: body.title ?? '', parent_page_id: body.parent_page_id ?? null, order_key: 'z', version: 1 };
      written.boards.push(made);
      PD_BOARDS.push(made);
      return route.fulfill({ status: 201, json: boardRead(made) });
    }
    if (/\/api\/v1\/collections\/[^/]+\/sprints$/.test(path)) {
      const at = /collections\/([^/]+)\/sprints/.exec(path)[1];
      return route.fulfill({ json: { data: SPRINTS[at] ?? [], runs_sprints: at in SPRINTS } });
    }
    // The sprint and the backlog views: one entry waiting in the backlog, the sprint without a burndown yet.
    if (/\/api\/v1\/collections\/[^/]+\/sprint-backlog$/.test(path)) return route.fulfill({ json: { data: [{ item_id: id(650), title: 'Offline mode', estimate: null }] } });
    if (/\/api\/v1\/collections\/[^/]+\/velocity$/.test(path)) return route.fulfill({ json: { unit: 'POINTS', average: 0, sprints: [] } });
    const running = Object.values(SPRINTS).flat().find((each) => path.endsWith(`/api/v1/sprints/${each.id}`));
    if (running) return route.fulfill({ json: { sprint: running, entries: sprintEntries(running.collection_id), burndown: [] } });
    const single = /\/api\/v1\/containers\/([^/:]+)$/.exec(path);
    const place = single && [...HUBS, ...COLLECTIONS].find((each) => each.id === single[1]);
    if (place && request.method() === 'GET') return route.fulfill({ json: place });
    if (path.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: HUBS } });
    if (path.endsWith('/api/v1/containers')) return route.fulfill({ json: { ...PAGE, data: COLLECTIONS.filter((each) => each.parent_id === url.searchParams.get('parent_id')) } });
    // A plan's schedule, empty: the tool's and a collection's timeline open on it.
    if (/\/api\/v1\/plans\/[^/]+\/schedule$/.test(path)) return route.fulfill({ json: { zone: 'UTC', finish: null, entries: [], dependencies: [], conflicts: [], unscheduled: [], undated: [], critical_path: [] } });
    if (/\/api\/v1\/plans\/[^/]+\/baselines$/.test(path)) return route.fulfill({ json: { data: [] } });
    if (/\/api\/v1\/plans\/[^/]+\/tolerance$/.test(path)) return route.fulfill({ json: { time_tolerance_days: null, source: 'DEFAULT', locked_by: null, baseline_id: null, exception: null } });
    // A place's own lists, bare arrays; the workspace's templates (`/config/templates`) are a page.
    if (/\/(labels|buckets|views|templates|custom-fields|policies|members)$/.test(path) && !path.includes('/api/v1/config/')) return route.fulfill({ json: [] });
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
