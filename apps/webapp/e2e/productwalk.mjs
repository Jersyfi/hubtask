// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A product walked end to end as a person clicks through it: its module on, its own routes answered
// by a small in-memory backend the walk keeps, everything else by the shared fixture. What the
// browser said along the way is folded by scalewalk-errors - a console error, an uncaught error, a
// failed request and every answer of 400 or more - so a broken route, a refused list call or a
// dialog that never closes fails the walk instead of passing every gate.

import { ACCOUNT, INVITED, MANIFEST, OTHER, signedIn, stub } from './fixture.mjs';
import { fold, record } from './scalewalk-errors.mjs';

/**
 * A signed-in page whose `/api/v1` reads go to `answer(method, path, query, body)` first: an
 * object `{ status, json }` (or `{ status }` for no body) is the answer, undefined leaves the
 * request to the fixture. `modules` are switched on in the manifest.
 */
export async function productWalk(browser, { modules, answer, width = 1280, height = 900 }) {
  const { page, context, unstubbed } = await signedIn(browser, width, height, { timezoneId: 'UTC' });
  const events = [];
  record(page, events);
  const written = [];
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, modules_on: modules } });
    // The batch read of names a current server answers; the shared fixture refuses it on purpose,
    // to walk the older server's fallback, which would count as a refusal here.
    if (path === '/accounts:resolve' && method === 'POST') {
      const asked = new Set(request.postDataJSON()?.ids ?? []);
      return route.fulfill({ json: { data: [ACCOUNT, OTHER, INVITED].filter((each) => asked.has(each.id)) } });
    }
    let body;
    try { body = request.postDataJSON(); } catch { body = undefined; }
    if (method !== 'GET') written.push({ method, path, body });
    const answered = answer(method, path, url.searchParams, body);
    if (answered) {
      return answered.json === undefined
        ? route.fulfill({ status: answered.status ?? 204, body: '' })
        : route.fulfill({ status: answered.status ?? 200, json: answered.json });
    }
    return stub(route);
  });
  return {
    page, context, written, unstubbed,
    /** The errors seen so far, folded: an empty list is a clean walk. Warnings are left aside. */
    errors: () => fold(events).filter((row) => row.level === 'ERROR'),
    close: () => context.close(),
  };
}

/** A fresh identifier per object a walk's backend makes, in the fixture's range. */
export function ids(prefix) {
  let next = 0;
  return () => `01a0e2e0-0000-7000-8000-${prefix}${String((next += 1)).padStart(12 - prefix.length, '0')}`;
}
