// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A calendar feed written into its owner's calendar at Microsoft 365 or Google (UC-INT-16): the
// paths, what the provider sent back, and which pushes a feed has - pure, so that it is tested
// without a server.

import type { CalendarPush } from '@hubtask/sync-engine';

export const calendarPushesPath = '/integrations/calendar-pushes';
export const calendarPushPath = (pushId: string) => `${calendarPushesPath}/${pushId}`;

/** The page the provider sends the browser back to, which the server names as its redirect. */
export const CALENDAR_PUSH_RETURN = '/profile/calendars';

export type PushProvider = CalendarPush['provider'];

/** The providers in the order the screens offer them. */
export const PUSH_PROVIDERS: readonly PushProvider[] = ['MICROSOFT', 'GOOGLE'];

/**
 * What the provider sent back on the return page: its state with a code, or with its refusal. A
 * query that carries no state is no answer - a reader who opened the page themselves.
 */
export function providerAnswerOf(query: Record<string, string>): { state: string; code?: string; error?: string } | undefined {
  const state = query['state'];
  if (!state) return undefined;
  const answer: { state: string; code?: string; error?: string } = { state };
  if (query['code']) answer.code = query['code'];
  if (query['error']) answer.error = query['error'];
  return answer;
}

/** A feed's pushes by provider: one per provider at most. */
export function pushesOf(pushes: readonly CalendarPush[], feedId: string): Partial<Record<PushProvider, CalendarPush>> {
  const out: Partial<Record<PushProvider, CalendarPush>> = {};
  for (const push of pushes) if (push.feed_id === feedId) out[push.provider] = push;
  return out;
}

/** The head's figure (user-experience.md §7.10): how many write, how many stopped on a refusal. */
export function pushFigure(list: readonly { readonly state: string }[]): { active: number; failed: number } {
  return {
    active: list.filter((each) => each.state === 'ACTIVE').length,
    failed: list.filter((each) => each.state === 'FAILED').length,
  };
}

/** The column's figure on *Calendars*: the pushes that stopped, nothing while every one writes. */
export function pushColumnFigure(
  list: readonly { readonly state: string }[],
  t: (code: string, params?: Record<string, string>) => string,
): string | undefined {
  const { failed } = pushFigure(list);
  return failed > 0 ? t('app.pushes.column_failed', { count: String(failed) }) : undefined;
}
