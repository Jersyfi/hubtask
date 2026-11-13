// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One shape for every vocabulary of states (work-surface concept §5, `StateLozenge`): status
// categories, SLA, goal, health, agent, rule, release and sync. The tone and the glyph of a state
// are decided here, once, so that "at risk" looks the same on a goal and on a deadline, and two
// products never draw one state in two shapes. The word is always the caller's resolved text; this
// table holds no sentence.

import type { IconName } from './icons/index.ts';

/** The five status tones plus the brand's, for the one category that is the reader's own work. */
export type LozengeTone = 'neutral' | 'info' | 'success' | 'warning' | 'danger' | 'brand';

/** A state as the lozenge draws it: a tone and, where the tone alone would carry meaning, a glyph. */
export interface LozengeLook {
  readonly tone: LozengeTone;
  readonly icon?: IconName;
}

/**
 * Every vocabulary and its states. A neutral state carries no glyph because it means nothing
 * beyond its word; every state whose tone says something also says it with a mark (UX-7, colour
 * never alone).
 */
export const LOZENGE_VOCABULARIES = {
  /** The three status categories a workflow's statuses fall into (the board's columns, L1/L2). */
  category: {
    todo: { tone: 'neutral' },
    progress: { tone: 'info' },
    done: { tone: 'success', icon: 'check' },
  },
  /** A deadline of an SLA or OLA. */
  sla: {
    running: { tone: 'neutral', icon: 'clock' },
    atRisk: { tone: 'warning', icon: 'triangle-alert' },
    breached: { tone: 'danger', icon: 'circle-alert' },
    paused: { tone: 'neutral', icon: 'pause' },
    met: { tone: 'success', icon: 'check' },
  },
  /** A goal's or a key result's confidence. */
  goal: {
    onTrack: { tone: 'success', icon: 'check' },
    atRisk: { tone: 'warning', icon: 'triangle-alert' },
    offTrack: { tone: 'danger', icon: 'circle-alert' },
    done: { tone: 'success', icon: 'circle-check' },
    notStarted: { tone: 'neutral' },
  },
  /** A configuration item's health. */
  health: {
    healthy: { tone: 'success', icon: 'check' },
    degraded: { tone: 'warning', icon: 'triangle-alert' },
    down: { tone: 'danger', icon: 'circle-alert' },
    unknown: { tone: 'neutral' },
    retired: { tone: 'neutral', icon: 'archive' },
  },
  /** An agent's standing. */
  agent: {
    draft: { tone: 'neutral' },
    approved: { tone: 'success', icon: 'check' },
    live: { tone: 'info', icon: 'zap' },
    paused: { tone: 'neutral', icon: 'pause' },
    failing: { tone: 'danger', icon: 'circle-alert' },
  },
  /** An automation rule's standing. */
  rule: {
    on: { tone: 'success', icon: 'check' },
    off: { tone: 'neutral' },
    needsLook: { tone: 'warning', icon: 'triangle-alert' },
    failing: { tone: 'danger', icon: 'circle-alert' },
  },
  /** A release. */
  release: {
    planned: { tone: 'neutral' },
    inProgress: { tone: 'info' },
    released: { tone: 'success', icon: 'check' },
    late: { tone: 'warning', icon: 'triangle-alert' },
    archived: { tone: 'neutral', icon: 'archive' },
  },
  /** A week of recorded time: open to record, handed in, approved, sent back (scene time). */
  timesheet: {
    open: { tone: 'neutral', icon: 'pencil' },
    submitted: { tone: 'info', icon: 'send' },
    approved: { tone: 'success', icon: 'check' },
    rejected: { tone: 'warning', icon: 'rotate-ccw' },
    locked: { tone: 'neutral', icon: 'lock' },
  },
  /** The device's synchronisation, as `SyncStatus` says it in a row. */
  sync: {
    synced: { tone: 'success', icon: 'cloud-check' },
    pending: { tone: 'neutral', icon: 'cloud-upload' },
    offline: { tone: 'warning', icon: 'cloud-off' },
    refused: { tone: 'danger', icon: 'circle-alert' },
  },
} as const satisfies Record<string, Record<string, LozengeLook>>;

export type LozengeVocabulary = keyof typeof LOZENGE_VOCABULARIES;
export type LozengeState<V extends LozengeVocabulary> = keyof (typeof LOZENGE_VOCABULARIES)[V];

/**
 * The look of one state of one vocabulary. An unknown state - a status category a newer server
 * sends - falls back to neutral rather than to nothing, so the word still stands.
 */
export function lozengeLook(vocabulary: LozengeVocabulary, state: string): LozengeLook {
  const table = LOZENGE_VOCABULARIES[vocabulary] as Record<string, LozengeLook>;
  return table[state] ?? { tone: 'neutral' };
}
