// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// A workspace the shell's walks share: one hub, one collection with entries, labels, buckets and
// members, and the manifest that declares them - stubbed at the network edge, the way the
// engines walk stubs its own. Nothing here is a claim about the product; it is the least a
// collection can be and still show every control the inventory names.
export const ACCOUNT = { id: '01a0e2e0-0000-7000-8000-000000000001', kind: 'USER', display_name: 'Jérôme Winkel', email: 'j@example.invalid', status: 'ACTIVE', locale: 'en', onboarding_completed_at: '2026-09-01T00:00:00Z' };
export const OTHER = { id: '01a0e2e0-0000-7000-8000-000000000011', kind: 'USER', display_name: 'Mara Lind', email: 'm@example.invalid', status: 'ACTIVE', locale: 'en' };
export const HUB = { id: '01a0e2e0-0000-7000-8000-000000000002', type: 'HUB', parent_id: null, name: 'House', order_key: 'a0', version: 1 };
export const COLLECTION = { id: '01a0e2e0-0000-7000-8000-000000000003', type: 'COLLECTION', parent_id: HUB.id, name: 'Kitchen', description: 'The renovation, room by room.', order_key: 'a0', version: 1 };
/** One that was put aside: out of the tree, and the only thing the archive screen should find. */
export const ARCHIVED = { id: '01a0e2e0-0000-7000-8000-000000000004', type: 'COLLECTION', parent_id: HUB.id, name: 'Last winter', order_key: 'a1', version: 1, archived_at: '2026-08-01T09:00:00Z' };
export const LABELS = [
  { id: '01a0e2e0-0000-7000-8000-000000000005', name: 'Materials', color_token: 'label.teal' },
  { id: '01a0e2e0-0000-7000-8000-000000000006', name: 'Urgent', color_token: 'label.red' },
];
export const BUCKETS = [
  { id: '01a0e2e0-0000-7000-8000-000000000021', collection_id: COLLECTION.id, name: 'To do', order_key: 'a0', wip_limit: null, is_done_bucket: false, version: 1 },
  { id: '01a0e2e0-0000-7000-8000-000000000022', collection_id: COLLECTION.id, name: 'Doing', order_key: 'a1', wip_limit: 2, is_done_bucket: false, version: 1 },
  { id: '01a0e2e0-0000-7000-8000-000000000023', collection_id: COLLECTION.id, name: 'Done', order_key: 'a2', wip_limit: null, is_done_bucket: true, version: 1 },
];
const item = (n, title, extra = {}) => ({ id: `01a0e2e0-0000-7000-8000-0000000000${n}`, type: 'TASK', title, collection_id: COLLECTION.id, parent_id: null, depth: 0, order_key: `a${n}`, version: 1, completion: { is_completed: false }, labels: [], label_ids: [], ...extra });
export const ITEMS = [
  item('31', 'Order the tiles for the splashback and the floor', { label_ids: [LABELS[0].id], labels: [LABELS[0]], bucket_id: BUCKETS[0].id }),
  item('32', 'Book the electrician', { label_ids: [LABELS[1].id], labels: [LABELS[1]], bucket_id: BUCKETS[1].id, due_at: '2026-09-25T09:00:00Z' }),
  item('33', 'Measure the worktop', { bucket_id: BUCKETS[0].id }),
  item('34', 'Choose the paint', { bucket_id: null }),
  item('35', 'Clear the room', { bucket_id: BUCKETS[2].id, completion: { is_completed: true } }),
];
/** The subtree under the first task: two work packages, one of them holding two activities. */
export const CHILDREN = {
  [ITEMS[0].id]: [
    { ...item('41', 'Tiles for the floor', { type: 'WORK_PACKAGE', parent_id: ITEMS[0].id, depth: 1 }) },
    { ...item('42', 'Tiles for the splashback', { type: 'WORK_PACKAGE', parent_id: ITEMS[0].id, depth: 1, completion: { is_completed: true } }) },
  ],
  '01a0e2e0-0000-7000-8000-000000000041': [
    { ...item('51', 'Measure the floor', { type: 'ACTIVITY', parent_id: '01a0e2e0-0000-7000-8000-000000000041', depth: 2, completion: { is_completed: true } }) },
    { ...item('52', 'Order the floor tiles', { type: 'ACTIVITY', parent_id: '01a0e2e0-0000-7000-8000-000000000041', depth: 2 }) },
  ],
  '01a0e2e0-0000-7000-8000-000000000042': [],
};
export const ALL_ITEMS = [...ITEMS, ...Object.values(CHILDREN).flat()];

export const MANIFEST = {
  product_version: '0.9.0', api_version: 'v1', tenancy_mode: 'single',
  // The three profiles as `domain-model.md` §2 defines them and a real server answers them,
  // spelling included — the fixture said `ASSIGNEE` for `ASSIGNMENT` and gave a work package two
  // capabilities of fourteen, which is a fake that agrees with no installation.
  item_types: [
    { type: 'TASK', capabilities: ['COMPLETION', 'DUE_DATE', 'REMINDER', 'ASSIGNMENT', 'MEMBERS', 'BUCKET', 'NOTES', 'LABELS', 'COMMENTS', 'COVER', 'ATTACHMENTS', 'HISTORY', 'RECURRENCE', 'CUSTOM_FIELDS'], allowed_child_types: ['WORK_PACKAGE'], max_depth: 3 },
    { type: 'WORK_PACKAGE', capabilities: ['COMPLETION', 'DUE_DATE', 'REMINDER', 'ASSIGNMENT', 'MEMBERS', 'NOTES', 'LABELS', 'COMMENTS', 'ATTACHMENTS', 'HISTORY', 'CUSTOM_FIELDS'], allowed_child_types: ['ACTIVITY'], max_depth: 2 },
    { type: 'ACTIVITY', capabilities: ['COMPLETION', 'DUE_DATE', 'REMINDER', 'ASSIGNMENT', 'HISTORY'], allowed_child_types: [], max_depth: 1 },
  ],
  supported_locales: [{ locale: 'en', direction: 'ltr' }, { locale: 'de', direction: 'ltr' }],
  query_fields: [
    { field: 'title', operators: ['CONTAINS', 'EQ'], sortable: true, groupable: false, nullable: false },
    { field: 'due_at', operators: ['LT', 'GT', 'IS_NULL'], sortable: true, groupable: false, nullable: true },
    { field: 'bucket_id', operators: ['EQ', 'IN'], sortable: false, groupable: true, nullable: true },
  ],
  view_layouts: ['LIST_COLLAPSED', 'LIST_EXPANDED', 'KANBAN', 'TIMELINE'],
  roles: [{ role: 'OWNER', permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS'] }],
  limits: { max_bulk_operations: 100 }, features: {},
};
export const PAGE = { data: [], items: [], page: { next_cursor: null, has_more: false } };

export function stub(route) {
  const request = route.request();
  const url = new URL(request.url());
  const path = url.pathname;
  if (path.endsWith('/api/v1/stream')) return route.abort();
  if (path.endsWith('/api/v1/sync:snapshot')) {
    const records = [HUB, COLLECTION].map((c) => JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: c.id, container_id: c.parent_id, payload: c }));
    return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${records.join('\n')}\n{"cursor":"c-e2e"}\n` });
  }
  if (path.endsWith('/api/v1/sync:pull')) return route.fulfill({ json: { changes: [], cursor: 'c-e2e', has_more: false, tombstone_window_days: 90 } });
  if (path.endsWith('/api/v1/meta/capabilities')) return route.fulfill({ json: MANIFEST });
  if (path.endsWith('/api/v1/accounts/me')) return route.fulfill({ json: ACCOUNT });
  if (path.endsWith(`/api/v1/accounts/${ACCOUNT.id}`)) return route.fulfill({ json: ACCOUNT });
  if (path.endsWith(`/api/v1/accounts/${OTHER.id}`)) return route.fulfill({ json: OTHER });
  // `include_archived` is honoured, because it is the whole of what the archive screen asks and a
  // stub that ignored it would prove the screen works by handing it rows the product hides.
  if (path.endsWith('/api/v1/containers')) {
    const archived = url.searchParams.get('include_archived') === 'true';
    const rows = url.searchParams.get('type') === 'HUB' ? [HUB] : [COLLECTION, ...(archived ? [ARCHIVED] : [])];
    return route.fulfill({ json: { ...PAGE, data: rows } });
  }
  if (path.endsWith(`/api/v1/containers/${COLLECTION.id}`)) return route.fulfill({ json: COLLECTION });
  if (path.endsWith(`/api/v1/containers/${ARCHIVED.id}`)) return route.fulfill({ json: ARCHIVED });
  if (path.endsWith(`/api/v1/containers/${HUB.id}`)) return route.fulfill({ json: HUB });
  if (path.endsWith('/labels')) return route.fulfill({ json: LABELS });
  if (path.endsWith('/buckets')) return route.fulfill({ json: BUCKETS });
  if (path.endsWith('/api/v1/memberships')) {
    const scope = url.searchParams.get('scope_type');
    const rows = scope === 'COLLECTION' ? [{ id: 'm1', scope_type: 'COLLECTION', scope_id: COLLECTION.id, account_id: OTHER.id, role: 'MEMBER' }] : scope === 'HUB' ? [{ id: 'm2', scope_type: 'HUB', scope_id: HUB.id, account_id: ACCOUNT.id, role: 'OWNER' }] : [];
    return route.fulfill({ json: { ...PAGE, data: rows } });
  }
  if (path.endsWith('/api/v1/items:query')) {
    const body = request.postDataJSON();
    if (body.group_by) {
      const groups = [...BUCKETS.map((b) => b.id), null].map((key) => { const data = ITEMS.filter((i) => (i.bucket_id ?? null) === key); return { key, count: data.length, data, page: { next_cursor: null, has_more: false } }; });
      return route.fulfill({ json: { data: [], groups, page: { next_cursor: null, has_more: false }, total: ITEMS.length } });
    }
    if (body.scope?.item_id) {
      // One level, or the whole subtree in one answer - the anchor is not in its own subtree.
      const below = (id) => (CHILDREN[id] ?? []).flatMap((child) => [child, ...(body.scope.include_descendants ? below(child.id) : [])]);
      const data = below(body.scope.item_id);
      return route.fulfill({ json: { data, groups: [], page: { next_cursor: null, has_more: false }, total: data.length } });
    }
    return route.fulfill({ json: { data: ITEMS, groups: [], page: { next_cursor: null, has_more: false }, total: ITEMS.length } });
  }
  const one = ALL_ITEMS.find((each) => path.endsWith(`/api/v1/items/${each.id}`));
  if (one && request.method() === 'GET') return route.fulfill({ json: one });
  if (one && request.method() === 'PATCH') return route.fulfill({ json: { ...one, ...request.postDataJSON(), version: one.version + 1 } });
  if (path.match(/\/api\/v1\/items\/[^/]+:(complete|reopen)$/) && request.method() === 'POST') {
    const id = path.split('/').pop().split(':')[0];
    const target = ALL_ITEMS.find((each) => each.id === id);
    return route.fulfill({ json: { ...target, completion: { is_completed: path.endsWith(':complete') }, version: target.version + 1 } });
  }
  const covered = ALL_ITEMS.find((each) => path.endsWith(`/api/v1/items/${each.id}/cover`));
  if (covered && request.method() === 'PUT') return route.fulfill({ json: { ...covered, cover: request.postDataJSON(), version: covered.version + 1 } });
  if (covered && request.method() === 'DELETE') return route.fulfill({ json: { ...covered, cover: null, version: covered.version + 1 } });
  if (path.match(/\/api\/v1\/items\/[^/]+\/(reminders|attachments|comments|activity)$/)) return route.fulfill({ json: { ...PAGE, data: [] } });
  if (path.match(/\/api\/v1\/items\/[^/]+\/recurrence$/)) return route.fulfill({ status: 404, json: { code: 'recurrence.not_found' } });
  if (/\/(views|templates|custom-fields|policies|feeds)$/.test(path)) return route.fulfill({ json: [] });
  return fallback(route, request, path);
}

/**
 * The reads the frame makes on every screen, and a record of everything else.
 *
 * **Shapes, because a wrong one is silent.** The API answers an array where the client maps over
 * one and an object where it reads a field; a page envelope in either place puts an object through
 * `.find`, or `undefined` where a field belongs, and the screen throws *while rendering*. That is
 * a failure a fast machine hides: the next navigation usually wins the race, and the slower CI
 * runner is where the screen gets far enough to try. Measured before this was written, the silent
 * catch-all was answering `/quotas`, `/meta/health` and `/integrations/calendar-feeds` — three
 * reads the frame makes on *every* screen — in a shape none of them has.
 *
 * **A record, because a guess nobody notices is the whole problem.** Anything this fixture was
 * never asked for is still answered — a walk that died here would say less than one that finishes
 * — but its path is kept, and `signedIn` hands the record to the test. `unstubbed()` beside
 * `failures` is the assertion: the same shape the walks already use for what the page threw.
 *
 * Exported, so that a walk with a stub of its own ends with this rather than inventing a second
 * catch-all. Every one of them had the same wrong shapes for the same reason.
 */
export function fallback(route, request, path) {
  const read = path.replace(/^.*\/api\/v1/, '');

  // Bringing a container back, which is what the archive is for. Answered with the container it
  // makes rather than with a page: the record below found this being guessed at in the walk whose
  // subject it is.
  if (read === `/containers/${ARCHIVED.id}:unarchive`) {
    const { archived_at: _put, ...back } = ARCHIVED;
    return route.fulfill({ json: { ...back, version: ARCHIVED.version + 1 } });
  }
  if (read === '/search') return route.fulfill({ json: { ...PAGE, data: [] } });

  // The three writes the walks make and had been guessing at, each answered with the entry it
  // makes. Two of them are the subject of the walk that makes them - the timeline's drag sets a
  // due date, the board's carry reorders - so a guess here is a walk that cannot claim to have
  // exercised what it is named after.
  const due = ALL_ITEMS.find((each) => read === `/items/${each.id}/due`);
  if (due) return route.fulfill({ json: { ...due, ...request.postDataJSON(), version: due.version + 1 } });
  const ranked = ALL_ITEMS.find((each) => read === `/items/${each.id}:reorder`);
  if (ranked) return route.fulfill({ json: { ...ranked, ...request.postDataJSON(), version: ranked.version + 1 } });
  if (read === '/items' && request.method() === 'POST') {
    return route.fulfill({ status: 201, json: { ...ALL_ITEMS[0], id: '01a0e2e0-0000-7000-8000-0000000000ff', ...request.postDataJSON(), version: 1 } });
  }

  if (ARRAYS.has(read)) return route.fulfill({ json: [] });
  if (read === '/meta/health') return route.fulfill({ json: { status: 'ok', degraded_features: [] } });
  if (read === '/jumble/entries') return route.fulfill({ json: { data: [], next_cursor: null } });

  unstubbed.push(`${request.method()} ${read}`);
  return route.fulfill({ json: PAGE });
}

/**
 * The reads that answer a bare array rather than a page envelope.
 *
 * One list, because the mistake is one mistake: every walk that wrote its own had the same three
 * wrong. A walk that visits the administration meets most of them without ever naming one.
 */
const ARRAYS = new Set([
  '/quotas', '/groups', '/retention-policies', '/oauth/clients', '/oauth/grants',
  '/auth/service-accounts', '/auth/tokens', '/auth/sessions', '/sync/devices',
  '/backup-targets', '/backup-schedules', '/backups', '/integrations/webhooks',
  '/integrations/calendar-feeds',
]);

/** What the fixture was asked for and had no answer prepared for. Read through `signedIn`. */
let unstubbed = [];

/** Starts a fresh record. `signedIn` does it; a walk with its own context calls it itself. */
export function forgetUnstubbed() {
  unstubbed = [];
}

/** What was asked for and guessed at, without repeats. */
export function unstubbedSoFar() {
  return [...new Set(unstubbed)];
}

/** A signed-in tab at a width, with the failures the bundle throws collected. */
export async function signedIn(browser, width, height = 800) {
  const context = await browser.newContext({ viewport: { width, height } });
  await context.route('**/api/v1/**', stub);
  await context.addInitScript(() => { sessionStorage.setItem('hubtask.bearer', 'e2e-bearer'); sessionStorage.setItem('hubtask.refresh', 'e2e-refresh'); });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  forgetUnstubbed();
  return { context, page, failures, unstubbed: unstubbedSoFar };
}
