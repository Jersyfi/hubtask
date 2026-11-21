// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** Putting the open page in the trash and leaving it for its space's pages. */

export interface TrashSteps<T> {
  /** Forgets the page's pending saves; one fired after the delete is refused. */
  readonly drop: () => void;
  /** The delete; undefined when it was refused. */
  readonly trash: () => Promise<T | undefined>;
  readonly leave: (path: string) => void;
}

/**
 * The hub is read before the delete: the delete invalidates the open page, and a screen that read
 * it afterwards would find nothing and stay on the deleted page's address.
 */
export async function trashAndLeave<T>(page: { readonly hub_id: string }, steps: TrashSteps<T>): Promise<T | undefined> {
  const hubId = page.hub_id;
  steps.drop();
  const answer = await steps.trash();
  if (answer !== undefined) steps.leave(`/hubs/${hubId}/pages`);
  return answer;
}
