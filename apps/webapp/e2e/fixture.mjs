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
export const MANIFEST = {
  product_version: '0.9.0', api_version: 'v1', tenancy_mode: 'single',
  item_types: [
    { type: 'TASK', capabilities: ['COMPLETION', 'BUCKET', 'DUE_DATE', 'LABELS', 'ASSIGNEE', 'COMMENTS', 'ATTACHMENTS'], allowed_child_types: ['WORK_PACKAGE'], max_depth: 3 },
    { type: 'WORK_PACKAGE', capabilities: ['COMPLETION', 'DUE_DATE'], allowed_child_types: ['ACTIVITY'], max_depth: 3 },
    { type: 'ACTIVITY', capabilities: ['COMPLETION'], allowed_child_types: [], max_depth: 3 },
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
  if (path.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: [HUB] } });
  if (path.endsWith('/api/v1/containers')) return route.fulfill({ json: { ...PAGE, data: [COLLECTION] } });
  if (path.endsWith(`/api/v1/containers/${COLLECTION.id}`)) return route.fulfill({ json: COLLECTION });
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
    if (body.scope?.item_id) return route.fulfill({ json: { data: [], groups: [], page: { next_cursor: null, has_more: false }, total: 0 } });
    return route.fulfill({ json: { data: ITEMS, groups: [], page: { next_cursor: null, has_more: false }, total: ITEMS.length } });
  }
  if (/\/(views|templates|custom-fields|policies|feeds)$/.test(path)) return route.fulfill({ json: [] });
  return route.fulfill({ json: PAGE });
}

/** A signed-in tab at a width, with the failures the bundle throws collected. */
export async function signedIn(browser, width, height = 800) {
  const context = await browser.newContext({ viewport: { width, height } });
  await context.route('**/api/v1/**', stub);
  await context.addInitScript(() => { sessionStorage.setItem('hubtask.bearer', 'e2e-bearer'); sessionStorage.setItem('hubtask.refresh', 'e2e-refresh'); });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  return { context, page, failures };
}
