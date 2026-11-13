// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The badge tone of a goal's or a portfolio's status: on track and done are good, at risk warns,
 * off track is a danger, dropped and none say nothing. One mapping, so that the tree, the goal's
 * page, the portfolio's table and a dashboard's goals colour a status alike.
 */

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
