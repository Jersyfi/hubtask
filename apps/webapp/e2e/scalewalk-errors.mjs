// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What run R3 collects from a browser (test/load/PLAN.md): every console error and warning, every
// uncaught error, every request that failed and every answer of 400 or more, one row per message
// in the shape of test/load/harness.LogError, so the browser's rows sit beside the server's and
// PostgreSQL's in errors-<items>.json.

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
  page.on('requestfailed', (request) =>
    events.push({ level: 'ERROR', message: `request failed: ${request.method()} ${pathOf(request.url())} ${request.failure()?.errorText ?? ''}` }));
  page.on('response', (response) => {
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
