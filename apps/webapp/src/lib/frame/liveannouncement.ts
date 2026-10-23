// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the frame says out loud when the live connection changes, as a message code.
 *
 * Only a **return** is worth hearing: the first connection after a page loads is not a change the
 * reader saw happen, and announcing it put the same sentence in the live region of every page
 * opened, read before the account's language had arrived, so it stood there in English. Becoming
 * live again after a loss, losing it once it was had, and the device losing its network are said.
 */

import type { LiveState } from '../data/live.svelte.ts';

export function liveAnnouncement(previous: LiveState | undefined, next: LiveState, hasBeenLive: boolean): string | undefined {
  if (previous === undefined || previous === next) return undefined;
  if (next === 'offline') return 'app.live.device_offline';
  if (!hasBeenLive) return undefined;
  if (next === 'live') return 'app.live.became_live';
  if (next === 'reconnecting') return 'app.live.lost';
  return undefined;
}
