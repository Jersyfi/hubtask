// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The walks' answers, written down so that the contract can judge them. With WALK_RECORD naming a
// directory, every answer a walk fulfils for `/api/v1/` - the fixture's or a suite's own stub -
// becomes one line of `<directory>/<suite>.jsonl`; test/contract reads the lines against
// api/openapi.yaml. Without the variable nothing here runs.
//
// Playwright's own classes are patched rather than the fixture's entry points: a suite that
// launches its own browser or routes on its own context never passes through `signedIn`.

import { appendFileSync, mkdirSync, writeFileSync } from 'node:fs';
import { basename, join } from 'node:path';

import { chromium } from 'playwright';

const API = '/api/v1';

/**
 * One answer as the record holds it, or `undefined` for a request outside the API. A body that is
 * not text is kept as base64; one taken from a file or a fetched response is not read, and
 * `body_from` says which.
 */
export function answerOf(method, url, options = {}) {
  const { pathname } = new URL(url);
  const at = pathname.indexOf(`${API}/`);
  if (at < 0) return undefined;
  const headers = Object.fromEntries(Object.entries(options.headers ?? {}).map(([name, value]) => [name.toLowerCase(), value]));
  const answer = {
    method,
    path: pathname.slice(at + API.length),
    status: options.status ?? options.response?.status() ?? 200,
    content_type: options.contentType ?? headers['content-type'] ?? (options.json !== undefined ? 'application/json' : null),
    body: null,
  };
  if (options.json !== undefined) answer.body = JSON.stringify(options.json);
  else if (typeof options.body === 'string') answer.body = options.body;
  else if (options.body) answer.body_base64 = Buffer.from(options.body).toString('base64');
  else if (options.path) answer.body_from = 'path';
  else if (options.response) answer.body_from = 'response';
  return answer;
}

/** Writes each distinct answer of this process once: a walk asks the same read many times. */
function recorder(directory, suite) {
  const file = join(directory, `${suite}.jsonl`);
  mkdirSync(directory, { recursive: true });
  // Written empty at once: a suite that answers nothing still leaves a record that it ran.
  writeFileSync(file, '');
  const seen = new Set();
  return (answer) => {
    const line = JSON.stringify(answer);
    if (seen.has(line)) return;
    seen.add(line);
    appendFileSync(file, `${line}\n`);
  };
}

const patched = new WeakSet();

/** Replaces `name` on the prototype of `instance`, once per prototype. */
function patch(instance, name, replacement) {
  const prototype = Object.getPrototypeOf(instance);
  const original = prototype[name];
  if (typeof original !== 'function' || patched.has(original)) return;
  const wrapped = replacement(original);
  patched.add(wrapped);
  prototype[name] = wrapped;
}

function install(write) {
  // `unroute(url, handler)` finds a route by its handler, so the same handler is always wrapped
  // by the same function.
  const wrappers = new WeakMap();
  const wrap = (handler) => {
    if (typeof handler !== 'function') return handler;
    if (!wrappers.has(handler)) {
      wrappers.set(handler, function recorded(route, request) {
        patch(route, 'fulfill', (fulfill) => function recordedFulfill(options) {
          const answer = answerOf(this.request().method(), this.request().url(), options);
          if (answer) write(answer);
          return fulfill.call(this, options);
        });
        return handler(route, request);
      });
    }
    return wrappers.get(handler);
  };
  const routes = (owner) => {
    patch(owner, 'route', (route) => function recordedRoute(url, handler, options) {
      return route.call(this, url, wrap(handler), options);
    });
    patch(owner, 'unroute', (unroute) => function recordedUnroute(url, handler) {
      return unroute.call(this, url, wrap(handler));
    });
  };
  const context = (opened) => {
    routes(opened);
    for (const page of opened.pages()) routes(page);
    // Before `newPage` resolves and for a popup alike, so a page's own `route` is patched in time.
    opened.on('page', routes);
    return opened;
  };
  const browser = (launched) => {
    patch(launched, 'newContext', (newContext) => async function recordedContext(...options) {
      return context(await newContext.apply(this, options));
    });
    patch(launched, 'newPage', (newPage) => async function recordedPage(...options) {
      const page = await newPage.apply(this, options);
      context(page.context());
      return page;
    });
    for (const opened of launched.contexts()) context(opened);
    return launched;
  };
  // The three engines share one class, so this covers `firefox` and `webkit` too.
  for (const name of ['launch', 'connect', 'connectOverCDP']) {
    patch(chromium, name, (launch) => async function recordedLaunch(...options) {
      return browser(await launch.apply(this, options));
    });
  }
  patch(chromium, 'launchPersistentContext', (launch) => async function recordedPersistent(...options) {
    return context(await launch.apply(this, options));
  });
}

if (process.env.WALK_RECORD && process.argv[1]?.endsWith('.test.mjs')) {
  install(recorder(process.env.WALK_RECORD, basename(process.argv[1], '.test.mjs')));
}
