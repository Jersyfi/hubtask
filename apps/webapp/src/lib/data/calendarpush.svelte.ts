// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The writes of a calendar push (UC-INT-16).

import type { CalendarPush, CalendarPushAuthorization } from '@hubtask/sync-engine';

import { calendarPushPath, calendarPushesPath, type PushProvider } from './calendarpush.ts';
import { engine } from './engine.ts';

export const calendarPushes = {
  /** Starts the provider's sign-in: answers the page to send the browser to. */
  authorize(feedId: string, provider: PushProvider): Promise<CalendarPushAuthorization> {
    return engine.mutate<CalendarPushAuthorization>('POST', `${calendarPushesPath}:authorize`, { feed_id: feedId, provider }, { idempotencyKey: crypto.randomUUID() });
  },
  complete(answer: { state: string; code?: string; error?: string }): Promise<CalendarPush> {
    return engine.mutate<CalendarPush>('POST', `${calendarPushesPath}:complete`, answer, {
      idempotencyKey: crypto.randomUUID(), invalidates: [calendarPushesPath],
    });
  },
  disconnect(push: CalendarPush): Promise<void> {
    return engine.mutate<void>('DELETE', calendarPushPath(push.id), undefined, { invalidates: [calendarPushesPath] });
  },
};
