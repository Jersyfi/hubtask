// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Opening an office file in the workspace's document server (UC-MED-06, F-3).
 *
 * **The launch is open** (ADR-0104): the browser has to hand the session's token to the server,
 * either by a form posted into a frame - which widens the page's `frame-src` and `form-action` to
 * the chosen origin - or by a new tab with the token in its address. Until that is decided
 * `EDITOR_LAUNCH` stays off, *Bearbeiten* is not drawn, and `launchEditor` only asks the server
 * for the session and gives it to `hand`, the one place the chosen way will go.
 */

import type { MediaEditingSession } from '@hubtask/sync-engine';

import { engine } from '../data/engine.ts';

/** Whether *Bearbeiten* is drawn and the launch runs. Off until the owner decides the launch. */
export const EDITOR_LAUNCH = false;

/** Asks for an editing session - refused unless the reader may change what holds the file. */
export async function launchEditor(mediaId: string, hand: (session: MediaEditingSession) => void): Promise<void> {
  if (!EDITOR_LAUNCH) return;
  const session = await engine.mutate<MediaEditingSession>('POST', `/media/${encodeURIComponent(mediaId)}:edit`, {}, { invalidates: [] });
  hand(session);
}
