// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A connector's provider return that may belong on another host (identity.md §10.1).
 *
 * A mailbox, a calendar push and a file store are connected through a provider that returns to
 * this installation's own page for each. In multi-tenant operation that page is on the
 * installation's host, while the connection began on the workspace's host, where the session that
 * completes it lives. So before anything else - before the sign-in card, before the page's own
 * completion - the return asks `:locate` and, where the answer is another host, goes there with
 * the path and the query unchanged (`forwardTo`).
 *
 * Pure: whether this address is such a return is decided before any request leaves.
 */

import type { ConnectorLocate } from '@hubtask/sync-engine';

/** The return pages, as the server registers them with each provider. */
const RETURN_PAGES: Readonly<Record<string, ConnectorLocate['kind']>> = {
  '/administration/mail/oauth': 'MAILBOX',
  '/profile/calendars': 'CALENDAR_PUSH',
  '/profile/files': 'FILE_STORE',
};

/** A workspace identifier, then the handle: the shape the server mints (`shared.ReturnState`). */
const LOCATABLE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.[A-Za-z0-9_-]+$/;

/**
 * The request to locate this return, or `undefined` where this is no connector's return - another
 * page, no state, or a state that names no workspace (one minted before it did, which is finished
 * where it lands, as before).
 */
export function connectorReturnOf(path: string, search: string): ConnectorLocate | undefined {
  const kind = RETURN_PAGES[path.replace(/\/+$/, '')];
  if (kind === undefined) return undefined;
  const state = new URLSearchParams(search.startsWith('?') ? search.slice(1) : search).get('state') ?? '';
  if (!LOCATABLE.test(state)) return undefined;
  return { state, kind };
}
