// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the frame decides about a person's emergency notices (ADR-0073 §4, UC-ID-16 check 11):
 * when the notice may interrupt, what the bar's mark shows, and what a hub's row in the navigation
 * says. Told once at entry and confirmed; afterwards the mark and the hub carry it for thirty days,
 * and no view draws a banner for it.
 */

import type { EmergencyNotice } from '@hubtask/sync-engine';

import { isRunning, recentAccesses } from '../data/privatehubs.ts';

/** The notices still worth a word: those of the last thirty days, newest first as the server sends them. */
export function currentNotices(notices: readonly EmergencyNotice[], now: number = Date.now()): readonly EmergencyNotice[] {
  return recentAccesses(notices, now) as readonly EmergencyNotice[];
}

/** The notices that ask to be confirmed and were not yet shown since the app was opened. */
export function waitingNotices(
  notices: readonly EmergencyNotice[], seen: ReadonlySet<string>, now: number = Date.now(),
): readonly EmergencyNotice[] {
  return currentNotices(notices, now).filter((each) => each.acknowledged_at === null && !seen.has(each.id));
}

/** What the frame reads at one moment before the notice may interrupt. */
export interface Moment {
  /** Unconfirmed notices not yet shown since the app was opened. */
  readonly waiting: number;
  /**
   * The account read, and the workspace's read answered - so the starter, where it is offered,
   * has had its chance to open first.
   */
  readonly isSettled: boolean;
  /** A dialog, sheet, drawer, popover or menu is open. */
  readonly isLayerOpen: boolean;
  /** The person has typed into the field that holds the focus. */
  readonly isTyping: boolean;
  /** The base tour or a product's tour is on the screen. */
  readonly isTourOpen: boolean;
  /** The workspace's starter is open. */
  readonly isStarterOpen: boolean;
}

/**
 * Whether the notice may interrupt now: something waits, the frame has settled, and nothing else
 * holds the person - never over a layer they opened, never in the middle of typing, never over the
 * starter or a tour (user-experience.md §3.1: one thing at a time). Otherwise it waits for the
 * next moment none of that is true.
 */
export function mayInterrupt(moment: Moment): boolean {
  return moment.waiting > 0 && moment.isSettled && !moment.isLayerOpen && !moment.isTyping && !moment.isTourOpen && !moment.isStarterOpen;
}

/** The bar's mark for the notices: none, quiet while every one is confirmed, raised while one is not. */
export type NoticeState = 'none' | 'quiet' | 'unconfirmed';

/**
 * The mark's state from the notices alone. A confirmation that failed changed nothing on the
 * server, so the notice is still unconfirmed and the mark keeps its dot.
 */
export function noticeStateOf(notices: readonly EmergencyNotice[], now: number = Date.now()): NoticeState {
  const current = currentNotices(notices, now);
  if (current.length === 0) return 'none';
  return current.some((each) => each.acknowledged_at === null) ? 'unconfirmed' : 'quiet';
}

/** What a hub's row says for thirty days after an access: while the hour runs, and after it. */
export interface HubMark {
  readonly notice: EmergencyNotice;
  readonly isRunning: boolean;
}

/** The newest access to this hub of the last thirty days, or nothing. */
export function hubMarkOf(notices: readonly EmergencyNotice[], hubId: string, now: number = Date.now()): HubMark | undefined {
  const newest = currentNotices(notices, now)
    .filter((each) => each.hub_id === hubId)
    .reduce<EmergencyNotice | undefined>((best, each) => (!best || Date.parse(each.opened_at) > Date.parse(best.opened_at) ? each : best), undefined);
  return newest ? { notice: newest, isRunning: isRunning(newest, now) } : undefined;
}

/**
 * Confirms each notice with `send`, one call per access, and answers the first refusal or nothing.
 * Every call is made even when one fails: a refusal of one leaves the others confirmed, and the
 * re-read after it shows which still wait.
 */
export async function confirmEach(ids: readonly string[], send: (id: string) => Promise<unknown>): Promise<unknown> {
  const results = await Promise.allSettled(ids.map((id) => send(id)));
  const refused = results.find((each): each is PromiseRejectedResult => each.status === 'rejected');
  return refused?.reason;
}

/** From this length the dialog adds that the reason is the opener's own text, in full. */
export const LONG_REASON = 200;
/** From this length a reason in the list of several folds to two lines, with a way to read it whole. */
export const FOLDED_REASON = 90;
