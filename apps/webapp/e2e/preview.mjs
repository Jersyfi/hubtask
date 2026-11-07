// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The built web app over the walks' fixture, for a person to look at in their own browser:
// `pnpm --filter @hubtask/webapp build`, then `node apps/webapp/e2e/preview.mjs` (`PORT=` a free port,
// `KOMPASS=1` the navigation concept's workspace).
//
// It answers "does this look right" without Go, PostgreSQL or a password: any email and password
// sign in, and every read is the fixture the engine walks use. It proves nothing about behaviour -
// a change that writes, syncs or authorises is walked against a real server
// (docs/evidence/README.md, "How to walk").
import { createReadStream, existsSync, statSync } from 'node:fs';
import { createServer } from 'node:http';
import { extname, join, normalize } from 'node:path';
import { fileURLToPath } from 'node:url';
import { AI_MANIFEST, SERVICE_ACCOUNT, stub } from './fixture.mjs';

// `AI=1` opens the workspace with AI available and a service account to run rules as - what a look
// at the rule editor's *Describe it* needs. `KOMPASS=1` serves the navigation concept's workspace
// (kompassstub.mjs: Product development, IT operations, Household and eleven more hubs, their
// queues, sprints, goals and contents) instead. Everything else is the shared fixture.
const withAi = process.env.AI === '1';
const kompass = process.env.KOMPASS === '1' ? await import('./kompassstub.mjs') : undefined;
// The stub module serves the walks' copy on a port of its own; the preview needs only its reads.
await kompass?.served.close();
const kompassHandler = kompass?.stubFor({
  modulesOn: kompass.ALL,
  hubModules: {
    [kompass.PD.id]: ['WORKFLOWS', 'PLANNING', 'TIME', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD'],
    [kompass.IT.id]: ['WORKFLOWS', 'SERVICE_DESK', 'ASSETS', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD'],
    [kompass.HOME.id]: ['WORKFLOWS'],
  },
  stars: [{ kind: 'COLLECTION', id: kompass.WEB.id }, { kind: 'VIEW', id: kompass.QUEUES[0].view_id }],
  owner: true,
  // What the walks answer beyond the stub: an opened page, a release, a workload and a queue's request
  // (kompassscreens.test.mjs), and the move dialog's pre-check (kompassmove.test.mjs) - IT operations
  // with obstacles, every other hub with warnings only.
  extra: (route, request, url) => {
    const path = url.pathname;
    // A page opened, as kompassscreens.test.mjs answers it; a queue's one request.
    if (path.endsWith('/api/v1/items:query') && (request.postDataJSON()?.filter?.nodes ?? []).some((node) => node.field === 'sla_standing')) {
      const row = { id: kompass.id(1200), title: 'Printer on the 2nd floor does not print', collection_id: kompass.DESK.id, number: 41, created_by: kompass.id(1), assignee_id: null, status: null, completion: { is_completed: false } };
      return route.fulfill({ json: { data: [row], total: 1, page: { next_cursor: null, has_more: false } } }).then(() => true);
    }
    if (/\/api\/v1\/pages\/[^/]+\/labels$/.test(path)) return route.fulfill({ json: { labels: [] } }).then(() => true);
    if (/\/api\/v1\/pages\/[^/]+\/[a-z-]+$/.test(path)) return route.fulfill({ json: { data: [], page: { next_cursor: null, has_more: false } } }).then(() => true);
    const opened = /\/api\/v1\/pages\/([^/]+)$/.exec(path);
    if (opened && request.method() === 'GET') {
      const found = kompass.PD_PAGES.find((each) => each.id === opened[1]);
      if (!found) return false;
      const parent = kompass.PD_PAGES.find((each) => each.id === found.parent_id);
      const ancestors = parent ? [{ id: parent.id, title: parent.title }] : [];
      return route.fulfill({ json: { ...found, body: 'Welcome to the team.', writable: false, labels: [], shared: false, drafts: false, unpublished: false, draft: null, ancestors, updated_at: '2026-11-06T10:00:00Z' } }).then(() => true);
    }
    // A release opened and a plan's workload, in the contract's shapes rather than a list's.
    const release = /\/api\/v1\/releases\/([^/]+)$/.exec(path);
    if (release && request.method() === 'GET') {
      const found = kompass.RELEASES.find((each) => each.id === release[1]);
      if (found) return route.fulfill({ json: { description: '', released_at: null, notes_page_id: null, ...found, entries: [], done_count: 0, open_count: 0 } }).then(() => true);
    }
    const plan = /\/api\/v1\/plans\/([^/]+)\/workload$/.exec(path);
    if (plan) return route.fulfill({ json: { plan_id: plan[1], aggregated: false, cells: [], team: [], unscheduled_minutes: 0 } }).then(() => true);
    // A week's timesheet and the other hubs' inventory health, which the shared stub answers in
    // older or list shapes.
    const week = /\/api\/v1\/timesheets\/(\d{4}-\d{2}-\d{2})$/.exec(path);
    if (week && request.method() === 'GET') return route.fulfill({ json: { week: week[1], state: 'OPEN', minutes: 0, records: [], days: [] } }).then(() => true);
    if (/\/api\/v1\/hubs\/[^/]+\/cmdb-health$/.test(path) && !path.includes(kompass.IT.id)) return route.fulfill({ json: { total: 0, problems: [], review_overdue: 0 } }).then(() => true);
    if (!path.endsWith(':move-preview')) return false;
    const to = request.postDataJSON()?.target_parent_id;
    const { IT, PD, id } = kompass;
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
    }).then(() => true);
  },
}).handler;

const dist = fileURLToPath(new URL('../dist/', import.meta.url));
const port = Number(process.env.PORT ?? 5180);
const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.svg': 'image/svg+xml', '.woff2': 'font/woff2', '.png': 'image/png', '.ico': 'image/x-icon', '.webmanifest': 'application/manifest+json' };

if (!existsSync(join(dist, 'index.html'))) {
  console.error('apps/webapp/dist is missing - run `pnpm --filter @hubtask/webapp build` first');
  process.exit(1);
}

/** The fixture speaks Playwright's route; this is the least of one a plain request can be. */
function route(request, response, url, body) {
  // A promise, as Playwright's `fulfill` answers: a route that returns `undefined` for "the general
  // stub answers" (the structure and figure hubs) would otherwise answer twice.
  const send = ({ status = 200, contentType, body: text, json }) => {
    response.writeHead(status, { 'content-type': json === undefined ? contentType : 'application/json' });
    response.end(json === undefined ? text : JSON.stringify(json));
    return Promise.resolve();
  };
  return {
    request: () => ({ url: () => url.href, method: () => request.method, postDataJSON: () => (body ? JSON.parse(body) : null), headers: () => request.headers }),
    fulfill: send,
    abort: () => send({ status: 503, contentType: 'text/plain', body: '' }),
  };
}

createServer((request, response) => {
  const url = new URL(request.url, `http://${request.headers.host}`);
  if (url.pathname.startsWith('/api/v1/')) {
    // The stream stays open, so the bar reads the connection as live rather than reconnecting.
    if (url.pathname === '/api/v1/stream') {
      response.writeHead(200, { 'content-type': 'text/event-stream', 'cache-control': 'no-store' });
      response.write('retry: 3600000\n\n');
      return;
    }
    let body = '';
    request.on('data', (chunk) => { body += chunk; });
    request.on('end', () => {
      if (url.pathname.startsWith('/api/v1/auth/sessions') && request.method === 'POST') {
        response.writeHead(201, { 'content-type': 'application/json' });
        response.end(JSON.stringify({ access_token: 'preview-bearer', refresh_token: 'preview-refresh', token_type: 'Bearer', expires_in: 3600 }));
        return;
      }
      const routed = route(request, response, url, body);
      if (kompassHandler) return void kompassHandler(routed);
      if (withAi && url.pathname === '/api/v1/meta/capabilities') return routed.fulfill({ json: AI_MANIFEST });
      if (withAi && url.pathname === '/api/v1/auth/service-accounts') return routed.fulfill({ json: [SERVICE_ACCOUNT] });
      stub(routed);
    });
    return;
  }
  const file = normalize(join(dist, decodeURIComponent(url.pathname)));
  const target = file.startsWith(dist) && existsSync(file) && statSync(file).isFile() ? file : join(dist, 'index.html');
  response.writeHead(200, { 'content-type': types[extname(target)] ?? 'application/octet-stream' });
  createReadStream(target).pipe(response);
}).listen(port, '127.0.0.1', () => {
  console.log(`web app preview on http://localhost:${port} - any email and password sign in`);
});
