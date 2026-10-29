// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A hub's whiteboards: the paths the *Whiteboards* screen reads, and the writes, so every write
 * invalidates the same set (UC-CNV-01 … UC-CNV-03). The reads are subscriptions at the call sites.
 *
 * **A scene write is not an edit of a form.** While people draw together the live document is the
 * board; the canvas saves the scene beside it without `If-Match`, the write landing on what stands,
 * so two writers' saves never refuse each other. A title is written with the board's version.
 */

import type {
  Whiteboard, WhiteboardChange, WhiteboardCreate, WhiteboardExport, WhiteboardRead, WhiteboardVersionList, WhiteboardVersionRead,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { liveBoardPath } from './whiteboards.ts';

export * from './whiteboards.ts';

export const boardsPath = (hubId: string) => `/hubs/${hubId}/whiteboards`;
export const trashedBoardsPath = (hubId: string) => `/hubs/${hubId}/whiteboards?trashed=true`;
export const boardPath = (boardId: string) => `/whiteboards/${boardId}`;

const touched = (hubId: string, boardId: string) => [boardPath(boardId), boardsPath(hubId), trashedBoardsPath(hubId)];

class Whiteboards {
  create(hubId: string, body: WhiteboardCreate): Promise<WhiteboardRead> {
    return engine.mutate<WhiteboardRead>('POST', boardsPath(hubId), body, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [boardsPath(hubId)],
    });
  }

  rename(board: Whiteboard, title: string): Promise<WhiteboardRead> {
    const change: WhiteboardChange = { title };
    return engine.mutate<WhiteboardRead>('PATCH', boardPath(board.id), change, {
      ifMatch: `"${board.version}"`,
      invalidates: touched(board.hub_id, board.id),
    });
  }

  /**
   * The canvas's scene, saved beside the live document. It invalidates the hub's list only: the
   * board itself is not read again, or the screen would draw a scene over the one being drawn.
   */
  saveScene(board: Whiteboard, elements: NonNullable<WhiteboardChange['elements']>): Promise<WhiteboardRead> {
    return engine.mutate<WhiteboardRead>('PATCH', boardPath(board.id), { elements }, {
      invalidates: [boardsPath(board.hub_id)],
    });
  }

  trash(board: Whiteboard): Promise<Whiteboard> {
    return engine.mutate<Whiteboard>('DELETE', boardPath(board.id), undefined, { invalidates: touched(board.hub_id, board.id) });
  }

  restore(board: Whiteboard): Promise<Whiteboard> {
    return engine.mutate<Whiteboard>('POST', `${boardPath(board.id)}:restore`, undefined, { invalidates: touched(board.hub_id, board.id) });
  }

  /** The native document of the board, or of one frame of it. */
  exportNative(boardId: string, frame?: string): Promise<WhiteboardExport> {
    return engine.mutate<WhiteboardExport>('POST', `${boardPath(boardId)}:export`, frame ? { frame } : {}, { invalidates: [] });
  }

  /** The board's history, newest first (UC-CNV-06). */
  async versions(boardId: string): Promise<WhiteboardVersionList['data']> {
    const state = await engine.refresh<WhiteboardVersionList>({ path: `${boardPath(boardId)}/versions` });
    if (state.status === 'failed') throw state.error;
    return state.status === 'ready' ? state.data.data : [];
  }

  /** One version with its objects, to bring back. */
  async version(boardId: string, seq: number): Promise<WhiteboardVersionRead> {
    const state = await engine.refresh<WhiteboardVersionRead>({ path: `${boardPath(boardId)}/versions/${seq}` });
    if (state.status !== 'ready') throw state.status === 'failed' ? state.error : new Error('version');
    return state.data;
  }

  nameVersion(boardId: string, seq: number, name: string): Promise<{ seq: number; name: string | null }> {
    return engine.mutate('PATCH', `${boardPath(boardId)}/versions/${seq}`, { name }, { invalidates: [`${boardPath(boardId)}/versions`] });
  }

  /** The board's live document socket; the connection asks for a new one on every reconnect. */
  liveSocket(boardId: string): WebSocket {
    return engine.liveSocket<WebSocket>(liveBoardPath(boardId));
  }
}

export const whiteboards = new Whiteboards();
