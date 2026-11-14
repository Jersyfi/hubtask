// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One hub's inventory as the scenes of work-surface §6.10 show it - the mockups' *IT-Service-Desk*:
// the service *Kundenportal* resting on *Portal-API* and *Portal-Frontend*, the servers below them,
// an incident on db-01, a change on the VPN gateway, the upkeep's problems, a round, connectors with
// their runs, and the register's five devices. The walks of the map, the upkeep and the register
// read it through the API's paths, each answer in the contract's shape.

import { ACCOUNT, HUB, MANIFEST, OTHER, PAGE, signedIn, stub } from './fixture.mjs';

export const id = (n) => `01a0e2e0-0000-7000-8000-0000000e${String(n).padStart(4, '0')}`;

const cls = (key, name, lineage, extra = {}) => ({
  key, name, parent: lineage[1] ?? '', system: false, lineage, version: 1, attributes: [], identifier_rules: [], required_relations: [],
  staleness_days: 0, all_attributes: [], ...extra,
});
export const CLASSES = [
  cls('ci', 'Object', ['ci'], { abstract: true, staleness_days: 60 }),
  cls('service', 'Services', ['service', 'ci'], { abstract: true }),
  cls('business_service', 'Business service', ['business_service', 'service', 'ci']),
  cls('application_service', 'Application service', ['application_service', 'service', 'ci']),
  cls('hardware', 'Hardware', ['hardware', 'ci'], { abstract: true }),
  cls('server', 'Server', ['server', 'hardware', 'ci']),
  cls('laptop', 'Laptop', ['laptop', 'hardware', 'ci']),
  cls('phone', 'Phone', ['phone', 'hardware', 'ci']),
  cls('location', 'Locations', ['location', 'ci'], { abstract: true }),
  cls('room', 'Room', ['room', 'location', 'ci']),
  cls('contract', 'Contract', ['contract', 'ci']),
];

const ci = (n, klass, name, extra = {}) => ({
  id: id(n), hub_id: HUB.id, class: klass, name, stage: 'IN_USE', status: 'OK', attributes: {}, provenance: { name: { source: 'glpi', at: '2026-11-14T06:00:00Z' } },
  protection: {}, quantity: 1, created_at: '2026-10-01T08:00:00Z', updated_at: '2026-10-01T08:00:00Z', version: 1, owner_account_id: ACCOUNT.id, ...extra,
});
const manual = { name: { source: 'manual', at: '2026-10-01T08:00:00Z' } };
export const CIS = {
  portal: ci(1, 'business_service', 'Kundenportal'),
  api: ci(2, 'application_service', 'Portal-API'),
  front: ci(3, 'application_service', 'Portal-Frontend'),
  db: ci(4, 'server', 'db-01.hh.intern'),
  app: ci(5, 'server', 'app-01', { stage: 'MAINTENANCE' }),
  lb: ci(6, 'server', 'lb-01.hh.intern'),
  rack: ci(7, 'room', 'RZ Hamburg · Rack 4', { provenance: manual }),
  app2: ci(8, 'server', 'app-02.hh.intern'),
  vpn: ci(9, 'server', 'VPN-Gateway Frankfurt'),
  mail: ci(10, 'business_service', 'E-Mail & Kalender', { provenance: manual }),
  exchange: ci(11, 'application_service', 'Exchange Online Connector', { provenance: manual }),
  nb0417: ci(20, 'laptop', 'ThinkPad T14'),
  nb0422: ci(21, 'laptop', 'ThinkPad T14', { owner_account_id: null, provenance: manual }),
  nb0431: ci(22, 'laptop', 'MacBook Air'),
  tl0102: ci(23, 'phone', 'Pixel 9'),
  nb0233: ci(24, 'laptop', 'ThinkPad T490', { stage: 'RETIRED' }),
  lager: ci(30, 'room', 'Lager HH'),
  frame: ci(31, 'contract', 'Rahmenvertrag Hardware 2025'),
};
const ALL_CIS = Object.values(CIS);

// The map: the service at layer 0, its application services at 2, what they stand on at 5.
const node = (each, layer) => ({ ci_id: each.id, class: each.class, name: each.name, stage: each.stage, layer });
const edge = (from, to, type = 'depends_on') => ({ source: from.id, target: to.id, type });
const PORTAL_NODES = [
  node(CIS.portal, 0), node(CIS.api, 2), node(CIS.front, 2), node(CIS.db, 5), node(CIS.app, 5), node(CIS.lb, 5), node(CIS.rack, 5), node(CIS.app2, 5), node(CIS.vpn, 5),
];
const PORTAL_EDGES = [
  edge(CIS.portal, CIS.api), edge(CIS.portal, CIS.front), edge(CIS.api, CIS.db), edge(CIS.api, CIS.app, 'runs_on'),
  edge(CIS.front, CIS.lb, 'runs_on'), edge(CIS.db, CIS.rack, 'located_in'), edge(CIS.app, CIS.app2, 'clusters_with'), edge(CIS.lb, CIS.vpn),
];
export const HUB_MAP = { hub_id: HUB.id, nodes: [...PORTAL_NODES, node(CIS.mail, 0), node(CIS.exchange, 2)], edges: [...PORTAL_EDGES, edge(CIS.mail, CIS.exchange)], truncated: false };
const PORTAL_MAP = { hub_id: HUB.id, nodes: PORTAL_NODES, edges: PORTAL_EDGES, truncated: false };
const MAIL_MAP = { hub_id: HUB.id, nodes: [node(CIS.mail, 0), node(CIS.exchange, 2)], edges: [edge(CIS.mail, CIS.exchange)], truncated: false };

const work = (n, key, title, extra = {}) => ({
  item_id: id(n), key, title, entry_type: 'incident', collection_id: id(900), hub_id: HUB.id, status: { key: 'open', name: 'Open', category: 'TODO' },
  completed: false, start_at: null, due_at: null, reason: 'BENEATH', via: null, ...extra,
});
export const PORTAL_WORK = {
  ci_id: CIS.portal.id,
  data: [
    work(100, 'SA-12', 'Anmeldung bricht mit Gateway Timeout ab', { via: { ci_id: CIS.db.id, name: CIS.db.name, class: 'server' } }),
    work(101, 'SA-8', 'Kundenportal in Firefox nicht zu öffnen', { reason: 'NAMES' }),
    work(102, 'CHA-1', 'Zertifikatskette ergänzen', { entry_type: 'change', start_at: '2026-11-19T21:00:00Z', due_at: '2026-11-19T22:00:00Z', via: { ci_id: CIS.vpn.id, name: CIS.vpn.name, class: 'server' } }),
    work(103, 'CHA-2', 'Datenbank-Patch einspielen', { entry_type: 'change', start_at: '2026-11-21T21:00:00Z', due_at: '2026-11-21T23:00:00Z', via: { ci_id: CIS.db.id, name: CIS.db.name, class: 'server' } }),
    work(104, 'SA-3', 'Alte Störung', { completed: true, via: { ci_id: CIS.lb.id, name: CIS.lb.name, class: 'server' } }),
  ],
};
/** CHA-2 collides with a change on the same database from another service; CHA-1 with none. */
const COLLISION = { item_id: id(110), title: 'Speicher erweitern', handle: 'cha', key_prefix: 'CHA', number: 9, entry_type: 'change', ci_id: CIS.db.id, start_at: '2026-11-21T22:00:00Z', due_at: '2026-11-21T23:30:00Z', direct: true };

// The upkeep.
export const HEALTH = {
  total: ALL_CIS.length, completeness: 1, correctness: 0.92, stale: 0, orphans: 1, duplicates: 0, without_owner: 1, review_overdue: 1,
  classes: [],
  problems: [
    { ci_id: CIS.nb0422.id, class: 'laptop', kinds: ['NO_OWNER'] },
    { ci_id: CIS.exchange.id, class: 'application_service', kinds: ['ORPHAN'] },
    { ci_id: CIS.mail.id, class: 'business_service', kinds: ['REVIEW_OVERDUE'] },
  ],
  stale_relations: 0,
  services: [
    { ci_id: CIS.portal.id, class: 'business_service', name: CIS.portal.name, cis: 9, wanting: 1, score: 0.89 },
    { ci_id: CIS.mail.id, class: 'business_service', name: CIS.mail.name, cis: 2, wanting: 2, score: 0.95 },
  ],
  sources: [],
};
export const HOLD = {
  id: id(200), hub_id: HUB.id, run_id: id(300), source: 'glpi', reason: 'cmdb.hold_conflict',
  item: { ref: 'db-01', class: 'server', name: 'db-01.hh.intern', native_key: 'glpi-17', attributes: { ram_gb: 64 } }, created_at: '2026-11-14T06:00:00Z',
};
const counts = (extra = {}) => ({ items: 12, created: 0, updated: 1, unchanged: 11, held: 1, duplicates: 0, refused: 0, ...extra });
export const RUNS = [
  { id: id(300), hub_id: HUB.id, source: 'glpi', actor_id: ACCOUNT.id, started_at: '2026-11-14T06:00:00Z', dry_run: false, counts: counts() },
  { id: id(301), hub_id: HUB.id, source: 'intune', actor_id: ACCOUNT.id, started_at: '2026-11-14T06:05:00Z', dry_run: false, counts: counts({ items: 2, updated: 0, unchanged: 2, held: 0 }) },
];
const RUN_DETAIL = {
  ...RUNS[0],
  decisions: [
    { ref: 'db-01', outcome: 'HOLD', ci_id: CIS.db.id, class: 'server', rule: 'name', taken: [], kept: [{ attribute: 'ram_gb', by: 'manual' }], reason: 'cmdb.hold_conflict' },
    { ref: 'app-01', outcome: 'UPDATE', ci_id: CIS.app.id, class: 'server', rule: 'name', taken: ['os'], kept: [] },
  ],
};
const connector = (n, key, kind, source, extra = {}) => ({
  id: id(n), hub_id: HUB.id, key, kind, source, push: false, inventory: false, base_url: `https://${key}.example.org`, settings: {}, allowed_hosts: [], hosts: [],
  every_minutes: 1440, enabled: true, collection_id: null, has_credential: true, next_run_at: '2026-11-15T06:00:00Z', last_run_at: '2026-11-14T06:00:00Z',
  last_outcome: 'OK', last_error: '', failures: 0, created_at: '2026-10-01T08:00:00Z', updated_at: '2026-10-01T08:00:00Z', version: 1, ...extra,
});
export const CONNECTORS = [
  connector(400, 'GLPI', 'GLPI', 'glpi'),
  connector(401, 'Intune', 'INTUNE', 'intune', { next_run_at: '2026-11-15T06:05:00Z', last_run_at: '2026-11-14T06:05:00Z' }),
  connector(402, 'alerts', 'ALERTMANAGER', 'alertmanager', { push: true, every_minutes: 0, next_run_at: null, last_run_at: null, last_outcome: '' }),
];
export const ROUND = {
  id: id(500), hub_id: HUB.id, name: 'Bestätigungsrunde Q4', classes: [], due_by: null, due_on: '2026-11-30', review_months: 12, started_by: ACCOUNT.id,
  started_at: '2026-11-01T08:00:00Z', closed_at: null, version: 1, score: { total: 12, confirmed: 7, disputed: 2, pending: 3, overdue: 0, rate: 0.75 },
};
const PAST_ROUND = { ...ROUND, id: id(501), name: 'Bestätigungsrunde Q3', closed_at: '2026-09-30T08:00:00Z', started_at: '2026-09-01T08:00:00Z' };
export const PAIR = { id: id(600), hub_id: HUB.id, ci_ids: [CIS.app.id, CIS.app2.id], rule: 'name', created_at: '2026-11-14T06:00:00Z' };

// The register.
const asset = (each, tag, state, cost, warranty, extra = {}) => ({
  ci_id: each.id, hub_id: HUB.id, name: each.name, class: each.class, asset_tag: tag, cost_minor: cost, currency: 'EUR', purchased_on: '2025-03-14',
  supplier: 'Lieferant Bechtle', warranty_until: warranty, warranty_ended: false, contract_id: null, stockroom_id: null, assigned_to: null, request_id: null,
  state, disposal: null, updated_at: '2026-10-01T08:00:00Z', version: 1, ...extra,
});
export const ASSETS = [
  asset(CIS.nb0417, 'NB-0417', 'DEPLOYED', 148000, '2028-03-31', { assigned_to: OTHER.id }),
  asset(CIS.nb0422, 'NB-0422', 'DEPLOYED', 148000, '2027-03-20', { assigned_to: ACCOUNT.id, contract_id: CIS.frame.id, stockroom_id: CIS.lager.id }),
  asset(CIS.nb0431, 'NB-0431', 'IN_STOCK', 139000, '2028-09-30', { stockroom_id: CIS.lager.id }),
  asset(CIS.tl0102, 'TL-0102', 'ORDERED', 79900, null),
  asset(CIS.nb0233, 'NB-0233', 'RETIRED', 124000, '2024-01-31', { warranty_ended: true }),
];
const moves = (each) => {
  if (each.asset_tag !== 'NB-0422') return [];
  return [
    { state: 'REQUESTED', at: '2025-03-12T09:00:00Z', by: ACCOUNT.id, assigned_to: null, stockroom_id: null },
    { state: 'ORDERED', at: '2025-03-14T09:00:00Z', by: ACCOUNT.id, assigned_to: null, stockroom_id: null },
    { state: 'IN_STOCK', at: '2025-03-20T09:00:00Z', by: ACCOUNT.id, assigned_to: null, stockroom_id: CIS.lager.id },
    { state: 'DEPLOYED', at: '2025-03-21T09:00:00Z', by: ACCOUNT.id, assigned_to: ACCOUNT.id, stockroom_id: null },
  ];
};

/**
 * Signs in at the width asked on the app `origin` serves, answers the inventory's reads from the seed - as the hub's owner or,
 * with `role`, a member who may only read - and opens `path`. `written` collects every write.
 */
export async function openInventory(browser, origin, { width = 1280, height = 900, path, role = 'OWNER', scheme = 'light' }) {
  const { page, context, failures } = await signedIn(browser, width, height);
  await page.emulateMedia({ colorScheme: scheme });
  const written = [];
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const at = url.pathname.replace('/api/v1', '');
    const method = request.method();
    if (role !== 'OWNER') {
      if (at === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: ['READ'] } });
      if (at === '/memberships') {
        const scope = url.searchParams.get('scope_type');
        const rows = scope === 'HUB' ? [{ id: 'm2', scope_type: 'HUB', scope_id: HUB.id, account_id: ACCOUNT.id, role }] : scope === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role }] : [];
        return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: [] } });
      }
    }
    if (method !== 'GET') {
      written.push({ method, at, body: request.postDataJSON?.() ?? null });
      if (/^\/cis\/[^/]+\/asset:move$/.test(at)) {
        const moved = ASSETS.find((each) => at.includes(each.ci_id));
        return route.fulfill({ json: { ...moved, state: request.postDataJSON()?.state ?? moved.state, version: moved.version + 1, moves: moves(moved) } });
      }
      if (/\/ci-holds\/[^/]+:dismiss$/.test(at) || /\/ci-holds\/[^/]+$/.test(at)) return route.fulfill({ json: { id: HOLD.id, dismissed: true } });
      return stub(route);
    }
    if (at === '/cmdb/classes') return route.fulfill({ json: { data: CLASSES } });
    if (at === '/cmdb/relation-types') return route.fulfill({ json: { data: [] } });
    if (at === `/hubs/${HUB.id}/cis`) return route.fulfill({ json: { data: ALL_CIS, next: null } });
    const one = /^\/cis\/([^/]+)$/.exec(at);
    if (one) {
      const found = ALL_CIS.find((each) => each.id === one[1]);
      return found ? route.fulfill({ json: found }) : route.fulfill({ status: 404, json: { code: 'not_found' } });
    }
    if (at === `/hubs/${HUB.id}/cmdb-health`) return route.fulfill({ json: HEALTH });
    if (at === `/hubs/${HUB.id}/ci-alerts`) return route.fulfill({ json: { data: [] } });
    if (at === `/hubs/${HUB.id}/ci-work`) return route.fulfill({ json: { data: [{ ci_id: CIS.db.id, open: 1 }], truncated: false } });
    if (at === `/hubs/${HUB.id}/ci-groups`) return route.fulfill({ json: { data: [] } });
    if (at === `/hubs/${HUB.id}/service-map`) {
      const service = url.searchParams.get('service_id');
      return route.fulfill({ json: service === CIS.portal.id ? PORTAL_MAP : service === CIS.mail.id ? MAIL_MAP : HUB_MAP });
    }
    if (at === `/cis/${CIS.portal.id}/work`) return route.fulfill({ json: PORTAL_WORK });
    if (/^\/cis\/[^/]+\/work$/.test(at)) return route.fulfill({ json: { ci_id: at.split('/')[2], data: [] } });
    const entryCis = /^\/items\/([^/]+)\/cis$/.exec(at);
    if (entryCis) {
      const collisions = entryCis[1] === id(103) ? [COLLISION] : [];
      return route.fulfill({ json: { item_id: entryCis[1], cis: [], services: [], related: [], collisions } });
    }
    if (at === `/hubs/${HUB.id}/ci-holds`) return route.fulfill({ json: { data: [HOLD] } });
    if (at === `/hubs/${HUB.id}/ci-runs`) return route.fulfill({ json: { data: RUNS } });
    if (at === `/ci-runs/${id(300)}`) return route.fulfill({ json: RUN_DETAIL });
    if (at === `/hubs/${HUB.id}/ci-duplicates`) return route.fulfill({ json: { data: [PAIR] } });
    if (at === `/hubs/${HUB.id}/ci-campaigns`) return route.fulfill({ json: { data: [ROUND, PAST_ROUND] } });
    if (at === `/hubs/${HUB.id}/cmdb-connectors`) return route.fulfill({ json: { data: CONNECTORS } });
    if (at === `/hubs/${HUB.id}/assets`) {
      const state = url.searchParams.get('state');
      return route.fulfill({ json: { data: ASSETS.filter((each) => !state || each.state === state), next: null } });
    }
    const record = /^\/cis\/([^/]+)\/asset$/.exec(at);
    if (record) {
      const found = ASSETS.find((each) => each.ci_id === record[1]);
      return found ? route.fulfill({ json: { ...found, moves: moves(found) } }) : route.fulfill({ status: 404, json: { code: 'not_found' } });
    }
    if (at === `/hubs/${HUB.id}/software`) {
      return route.fulfill({ json: { data: [{ publisher: 'Microsoft', product: 'Office', aliased: true, installations: 3, versions: [{ version: '2024', cis: 2, installations: 2 }, { version: '2021', cis: 1, installations: 1 }] }] } });
    }
    return stub(route);
  });
  await page.goto(`${origin}${path}`);
  return { page, context, failures, written };
}

export const bar = (page) => page.getByRole('navigation', { name: 'Parts of the inventory' });
export async function noSidewaysScroll(page, what, assert) {
  const wide = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(wide <= 1, `${what} scrolls sideways by ${wide} px`);
}
