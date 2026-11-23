// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** Putting the open page in the trash and leaving it for its hub's *All contents*. */

export interface TrashSteps<T> {
  /**
   * Holds the page's pending saves (one fired after the delete is refused); what it returns gives
   * them back, called only when the delete was refused.
   */
  readonly drop: () => unknown;
  /** The delete; undefined when it was refused. */
  readonly trash: () => Promise<T | undefined>;
  readonly leave: (path: string) => void;
}

/**
 * Where a page, a post or a board put in the trash leaves the person who was on it: the hub's *All
 * contents*, narrowed to the kind it was (a post is not among them). Not the space's own address,
 * which opens its home page - the page just trashed, when that was the home.
 */
export const contentsAfterTrash = (hubId: string, kind?: 'pages' | 'whiteboards'): string =>
  `/hubs/${hubId}/contents${kind ? `?kind=${kind}` : ''}`;

/**
 * The hub is read before the delete: the delete invalidates the open page, and a screen that read
 * it afterwards would find nothing and stay on the deleted page's address.
 */
export async function trashAndLeave<T>(page: { readonly hub_id: string }, steps: TrashSteps<T>, kind?: 'pages'): Promise<T | undefined> {
  const hubId = page.hub_id;
  const restore = steps.drop();
  const answer = await steps.trash();
  if (answer !== undefined) steps.leave(contentsAfterTrash(hubId, kind));
  else if (typeof restore === 'function') restore();
  return answer;
}

/**
 * A trash pressed again while one is pending does nothing: the editor's saves are held by one
 * flag, and a refused first delete would give them back while an accepted second is still open.
 */
export function oneTrashAtATime<T>(run: () => Promise<T>): () => Promise<T | undefined> {
  let pending: Promise<T> | undefined;
  return async () => {
    if (pending) return undefined;
    pending = run();
    try {
      return await pending;
    } finally {
      pending = undefined;
    }
  };
}
