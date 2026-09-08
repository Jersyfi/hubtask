// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * One entry's comments: reading the thread, and the three writes C-03 gave it.
 *
 * **Oldest first, by cursor.** The API has no page numbers, so neither does this. `LoadMore`
 * appends, which is what the engine's `loadMore` does, and what a reader who pressed it expects.
 *
 * **A deletion is not a removal from the list.** The server keeps the row with its body gone, so
 * the thread is re-read rather than edited locally: the tombstone is the answer, and a client that
 * spliced the comment out would be disagreeing with the next page it asks for.
 *
 * **No comment text leaves in a URL.** A body travels in a request document, never in a query
 * string, for the reason a search term does (`security.md` §9): a query string reaches access
 * logs, proxies and browser history.
 */

import type { Comment, CommentPage } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';

/** Where the thread lives. One path, so two readers of the same entry share one read. */
export const commentsPath = (itemId: string) => `/items/${itemId}/comments`;

/**
 * A comment write touches the thread and nothing else.
 *
 * Not `/items`: neither adding nor editing nor deleting a comment moves the entry's own row, and
 * naming it would reload every list on screen to show a change none of them renders.
 */
const touches = (itemId: string) => [commentsPath(itemId)];

class Comments {
  /** Adds a comment, or a reply to one. A reply carries the parent; a comment carries none. */
  async add(itemId: string, body: string, parentCommentId: string | undefined, idempotencyKey: string): Promise<Comment> {
    return engine.mutate<Comment>(
      'POST',
      commentsPath(itemId),
      { body, parent_comment_id: parentCommentId ?? null },
      { idempotencyKey, invalidates: touches(itemId) },
    );
  }

  /**
   * Rewrites the text. `If-Match` carries the version the reader had, so an edit against a comment
   * that moved underneath them is refused as `version_conflict` rather than overwriting quietly
   * (ADR-0025) — the body is last-writer-wins and the displaced text is not kept anywhere.
   */
  async edit(itemId: string, comment: Comment, body: string): Promise<Comment> {
    return engine.mutate<Comment>(
      'PATCH',
      `${commentsPath(itemId)}/${comment.id}`,
      { body },
      { ifMatch: etagFor(comment.version), invalidates: touches(itemId) },
    );
  }

  /** Removes the text and marks it deleted. The row stays, so a reply under it does not dangle. */
  async remove(itemId: string, comment: Comment): Promise<void> {
    await engine.mutate<void>(
      'DELETE',
      `${commentsPath(itemId)}/${comment.id}`,
      undefined,
      { ifMatch: etagFor(comment.version), invalidates: touches(itemId) },
    );
  }
}

export const comments = new Comments();
export type { CommentPage };
