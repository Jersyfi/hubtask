// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The one request helper every trigger, create and search of the app calls (P-05, ADR-0058).
//
// The installation's address is part of the connection (`bundle.authData.base_url`): Hubtask is
// self-hosted, so there is no one host to write into the app, and the person connecting names
// theirs. The bearer is the OAuth2 access token Zapier holds for the connection; the platform
// adds it through the authentication's `Authorization` header before this runs, and refreshes
// it on a 401. A refusal becomes a `z.errors.Error` carrying the problem document's code, which
// is what the Zap's history shows.
'use strict';

const API_VERSION = 'v1';

function root(bundle) {
  return `${String(bundle.authData.base_url || '').replace(/\/+$/, '')}/api/${API_VERSION}`;
}

async function request(z, bundle, method, path, { body, query, headers } = {}) {
  const response = await z.request({
    method,
    url: root(bundle) + path,
    params: compact(query),
    headers: { Accept: 'application/json', ...compact(headers) },
    body: body === undefined ? undefined : body,
    skipThrowForStatus: true,
  });
  if (response.status >= 400) {
    const problem = response.data && typeof response.data === 'object' ? response.data : {};
    const code = problem.detail_code || problem.code || `http_${response.status}`;
    throw new z.errors.Error(`Hubtask refused the request: ${code}`, code, response.status);
  }
  return response.status === 204 ? {} : response.data;
}

/** Drops undefined and empty values, so that an optional input Zapier left blank is not sent. */
function compact(object) {
  const out = {};
  for (const [key, value] of Object.entries(object || {})) {
    if (value === undefined || value === null || value === '') continue;
    out[key] = value;
  }
  return out;
}

/** The URL for a webhook subscription's target, which Zapier hands the trigger. */
function targetUrl(bundle) {
  return bundle.targetUrl;
}

module.exports = { API_VERSION, request, compact, targetUrl };
