// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { StartCard } from '../product/workspaceStarts.svelte.ts';

/**
 * Time's start cards (scene emptytime) are its three ways to record, not its templates: the timer
 * on an entry, the week, and the billable kinds - the `time_consulting` template, where offered.
 */
export function timeWays(
  templates: readonly StartCard[],
  word: (code: string) => string,
  go: { timer: () => void; week: () => void },
): StartCard[] {
  const billable = templates.find((each) => each.key === 'time_consulting');
  return [
    { key: 'timer', label: word('app.time.way_timer'), description: word('app.time.way_timer_line'), icon: 'timer', onclick: go.timer },
    { key: 'week', label: word('app.time.way_week'), description: word('app.time.way_week_line'), icon: 'calendar', onclick: go.week },
    ...(billable ? [{ ...billable, label: word('app.time.way_billable'), description: word('app.time.way_billable_line') }] : []),
  ];
}
