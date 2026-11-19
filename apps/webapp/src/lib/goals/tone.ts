// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The badge tone of a goal's or a portfolio's status: on track and done are good, at risk warns,
 * off track is a danger, dropped and none say nothing. One mapping, so that the tree, the goal's
 * page, the portfolio's table and a dashboard's goals colour a status alike.
 */

import type { StatusCategory, StatusRef, WorkflowStep } from '@hubtask/design-system/components';

export type Tone = 'success' | 'warning' | 'danger' | 'info' | 'neutral';

/** A goal's or a portfolio's status in the lozenge's goal vocabulary, which adds the glyph to the tone. */
export function goalState(status: string | null | undefined): 'onTrack' | 'atRisk' | 'offTrack' | 'done' | 'notStarted' {
  switch (status) {
    case 'ON_TRACK':
      return 'onTrack';
    case 'AT_RISK':
      return 'atRisk';
    case 'OFF_TRACK':
      return 'offTrack';
    case 'DONE':
      return 'done';
    default:
      return 'notStarted';
  }
}

export function goalTone(status: string | null | undefined): Tone {
  switch (status) {
    case 'ON_TRACK':
    case 'DONE':
      return 'success';
    case 'AT_RISK':
      return 'warning';
    case 'OFF_TRACK':
      return 'danger';
    default:
      return 'neutral';
  }
}

/** A goal's or a portfolio's status as the head's status button lists it (H-2). */
export interface HealthMenu {
  readonly current: StatusRef;
  readonly statuses: readonly StatusRef[];
  /** One step per status but the current: choosing it opens the check-in or the report. */
  readonly steps: readonly WorkflowStep[];
}

/**
 * The status button's lists for a goal or a portfolio: every status a check-in or a status report
 * may give, the current one ticked. A status changes only through that check-in or report - the
 * button opens it with the status chosen, it never sets the status on its own.
 */
export function healthMenu(current: string, offered: readonly string[], word: (status: string) => string): HealthMenu {
  const category = (status: string): StatusCategory => (status === 'DONE' || status === 'DROPPED' ? 'done' : status === 'NONE' ? 'todo' : 'progress');
  const ref = (status: string): StatusRef => ({ id: status, label: word(status), category: category(status) });
  const statuses = offered.map(ref);
  return { current: ref(current), statuses, steps: statuses.filter((each) => each.id !== current).map((to) => ({ id: to.id, label: to.label, to })) };
}
