// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What run R3 collects from a browser (test/load/PLAN.md): every console error and warning, every
// uncaught error, every request that failed - not one cancelled by leaving the page - and every
// answer of 400 or more, one row per message in the shape of test/load/harness.LogError, so the
// browser's rows sit beside the server's and PostgreSQL's in errors-<items>.json.

// The parts of a message that differ between occurrences of one error, masked as the harness masks
// them: quoted values, identifiers and numbers.
const VARYING = /"[^"]*"|'[^']*'|[0-9a-f]{8}-[0-9a-f-]{27}|\b\d+\b/g;

/** Masks what varies in a message, so one error seen on many entries is one row. */
export function mask(message) {
  return String(message).replace(VARYING, '?').trim();
}

/**
 * Records a page's errors into `events`: { level, message } per occurrence. A URL's query and
 * fragment are dropped first - a failed request is its path, not its parameters.
 */
export function record(page, events) {
  page.on('console', (message) => {
    const level = message.type();
    if (level === 'error' || level === 'warning') events.push({ level: level === 'error' ? 'ERROR' : 'WARN', message: message.text() });
  });
  page.on('pageerror', (error) => events.push({ level: 'ERROR', message: `uncaught: ${error.message ?? error}` }));
  // Leaving a document cancels what it still had in flight - an idle chunk fetch, the stream - and
  // Playwright 1.65's Chromium reports each as net::ERR_ABORTED, between the next document's request
  // and its commit. The old document runs until that commit, so what it issues meanwhile is its own.
  // Only net::ERR_ABORTED, there or on a request of an earlier document, is left out. A navigation
  // that commits nothing ends the window as well: one that fails (a download), and one answered 204
  // or 205, which Chromium 156 may report as finished rather than failed.
  let document = 0;
  let leaving = null;
  const issuedIn = new WeakMap();
  const main = (request) => request.isNavigationRequest() && request.frame() === page.mainFrame();
  page.on('request', (request) => {
    if (main(request)) leaving = request;
    else issuedIn.set(request, document);
  });
  page.on('framenavigated', (frame) => {
    // A history change commits too, with no request before it: no document was left.
    if (frame !== page.mainFrame() || !leaving) return;
    leaving = null;
    document += 1;
  });
  page.on('requestfailed', (request) => {
    const errorText = request.failure()?.errorText ?? '';
    if (main(request)) {
      if (request === leaving) leaving = null;
    } else if (errorText === 'net::ERR_ABORTED' && (leaving || issuedIn.get(request) < document)) return;
    events.push({ level: 'ERROR', message: `request failed: ${request.method()} ${pathOf(request.url())} ${errorText}` });
  });
  page.on('response', (response) => {
    if (response.request() === leaving && (response.status() === 204 || response.status() === 205)) leaving = null;
    const event = answerEvent(response.status(), response.statusText(), response.request().method(), response.url());
    if (event) events.push(event);
  });
}

/**
 * An answer of 400 or more as an event, or null. A refusal is one too: the web app's own screens
 * never ask for what their person may not read, so a 403 on a walk is a finding with its path.
 */
export function answerEvent(status, statusText, method, url) {
  if (status < 400) return null;
  const words = status >= 500 ? 'answered' : 'refused';
  return { level: 'ERROR', message: `${words} ${status} ${statusText}: ${method} ${pathOf(url)}` };
}

function pathOf(url) {
  try {
    return new URL(url).pathname;
  } catch {
    return String(url);
  }
}

/** Folds the occurrences into one row per level and masked message, the most frequent first. */
export function fold(events) {
  const counted = new Map();
  for (const { level, message } of events) {
    const key = `${level}\u0000${mask(message)}`;
    counted.set(key, (counted.get(key) ?? 0) + 1);
  }
  return [...counted]
    .map(([key, count]) => {
      const [level, message] = key.split('\u0000');
      return { source: 'browser', level, message, count };
    })
    .sort((a, b) => b.count - a.count || a.message.localeCompare(b.message) || a.level.localeCompare(b.level));
}
