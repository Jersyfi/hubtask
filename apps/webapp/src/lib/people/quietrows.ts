// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which quiet rows the conversation's *All* draws beside the newest posts (ET-3, §6.9): only what
 * happened since the oldest post shown - the rest waits behind *Older comments* with its posts.
 * Without a post the newest three events stand alone.
 */
export const QUIET_WITHOUT_POSTS = 3;

export function quietRowsInView<E extends { readonly at: string }>(
  events: readonly E[],
  shown: readonly { readonly at: string }[],
): E[] {
  if (shown.length === 0) {
    return [...events].sort((a, b) => Date.parse(b.at) - Date.parse(a.at)).slice(0, QUIET_WITHOUT_POSTS);
  }
  const oldest = Math.min(...shown.map((post) => Date.parse(post.at)));
  return events.filter((event) => Date.parse(event.at) >= oldest);
}
