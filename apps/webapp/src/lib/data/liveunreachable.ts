// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Whether a page's or a board's live connection is out of reach while the rest of Hubtask answers:
// several sockets in a row closed before they opened, and the event stream - plain HTTP through the
// same proxy - is live. That is the mark of a proxy that does not pass the WebSocket upgrade
// (deployment.md §4), and the reader is told so rather than "trying again" for ever.

/** Consecutive connections that never opened before the cause is named. */
export const LIVE_UNREACHABLE_AFTER = 3;

export function isLiveUnreachable(status: string, failedAttempts: number, streamState: string): boolean {
  return status === 'offline' && failedAttempts >= LIVE_UNREACHABLE_AFTER && streamState === 'live';
}
