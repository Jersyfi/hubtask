// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The satisfaction question by mail for a requester without an account (UC-SVC-07 check 6): how
 * the rating page reads its link back.
 *
 * The resolution mail links `/rate#hbt_rate_….<score>`: the token and the score the requester
 * chose ride in the fragment, which never reaches a server log or a Referer, and opening the link
 * records nothing - the page asks once more, so a mail scanner that follows every link rates
 * nothing.
 */

const PREFIX = 'hbt_rate_';

/** The token and the chosen score a fragment carries; an empty token for anything that is not one. */
export function ratingLinkOf(fragment: string): { token: string; score: number | undefined } {
  const [token = '', score = ''] = fragment.replace(/^#/u, '').trim().split('.', 2);
  if (!token.startsWith(PREFIX) || !/^[A-Za-z0-9_-]+$/u.test(token)) return { token: '', score: undefined };
  const chosen = /^[1-5]$/u.test(score) ? Number(score) : undefined;
  return { token, score: chosen };
}

export const ratingPath = (token: string) => `/public/satisfaction/${encodeURIComponent(token)}`;
