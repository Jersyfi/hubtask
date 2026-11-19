// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Opening an office file in the workspace's document server (UC-MED-06, F-3, ADR-0104).
 *
 * The server answers a short-lived token for this person and this file and the editor's address
 * from the document server's discovery; the viewer posts the token there in a form into a frame,
 * never in a URL. The page's policy lets a form post and a frame show exactly that server's origin,
 * and only while the workspace has one entered.
 */

import type { MediaEditingSession } from '@hubtask/sync-engine';

import { engine } from '../data/engine.ts';

/** Asks for an editing session - refused unless the reader may change what holds the file. */
export function startEditing(mediaId: string): Promise<MediaEditingSession> {
  return engine.mutate<MediaEditingSession>('POST', `/media/${encodeURIComponent(mediaId)}:edit`, {}, { invalidates: [] });
}

/** The frame's name a session's form targets: one per file, so two viewers never share a frame. */
export function editorFrameName(mediaId: string): string {
  return `hubtask-editor-${mediaId}`;
}
