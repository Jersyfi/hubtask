// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Who else is on a board (UC-CNV-05 check 3): the others' awareness states read as people with a
// name, a cursor and the middle of their view, and the view of the one being followed.

import type { Point } from '@hubtask/whiteboard';

/** The middle of a person's view and its zoom: what a follower centres on. */
export interface PeerView { x: number; y: number; zoom: number }

export interface Peer { id: number; name: string; cursor?: Point; view?: PeerView }

/**
 * The others on the board, the reader's own state left out. A state without a name reads as
 * `someone`, so a person is never listed blank.
 */
export function peersOf(states: Map<number, unknown>, self: number, someone: string): Peer[] {
  const out: Peer[] = [];
  states.forEach((state, id) => {
    if (id === self || !state) return;
    const s = state as { name?: unknown; cursor?: Point; view?: PeerView };
    out.push({ id, name: typeof s.name === 'string' && s.name ? s.name : someone, cursor: s.cursor, view: s.view });
  });
  return out;
}

/**
 * Who is followed after the others changed: nobody once that person has left the board.
 */
export function stillFollowing(peers: Peer[], following: number | undefined): number | undefined {
  return following !== undefined && peers.some((p) => p.id === following) ? following : undefined;
}

/**
 * The follower's viewport centred where the followed person looks, at their zoom; undefined
 * where they have said nothing of their view yet.
 */
export function followedViewport(
  peers: Peer[], following: number | undefined, size: { width: number; height: number },
): { x: number; y: number; zoom: number } | undefined {
  const leader = following === undefined ? undefined : peers.find((p) => p.id === following);
  if (!leader?.view) return undefined;
  const zoom = leader.view.zoom;
  return { zoom, x: leader.view.x - size.width / zoom / 2, y: leader.view.y - size.height / zoom / 2 };
}
