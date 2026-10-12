// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** What the screens decide about emergency accesses, apart from the store that reads them. */

import type { PrivateHubAccess } from '@hubtask/sync-engine';

/** How long a notice of an access stays on the hub's page (the server's 30 days for a person's own). */
export const NOTICE_DAYS = 30;

/** The accesses a page still says something about: opened within the last thirty days. */
export function recentAccesses(
  accesses: readonly PrivateHubAccess[], now: number = Date.now(),
): readonly PrivateHubAccess[] {
  const since = now - NOTICE_DAYS * 24 * 60 * 60 * 1000;
  return accesses.filter((each) => Date.parse(each.opened_at) >= since);
}

/** Whether an access is still open at the moment. */
export function isRunning(access: PrivateHubAccess, now: number = Date.now()): boolean {
  return Date.parse(access.ends_at) > now;
}
