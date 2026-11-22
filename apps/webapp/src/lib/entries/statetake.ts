// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * One choice of the state control taken (build-plan §5): a column through the base product's
 * `PATCH bucket_id` - the server completes into the done column and reopens out of it (UC-WRK-05
 * check 6), with the Workflows module on or off -, a step through the step. The chip's menu, the
 * mark, a row's swipe and its menu's *advance* all come here, so they cannot drift apart.
 */

import type { WorkItem } from '@hubtask/sync-engine';

import { announcer } from '../announce.svelte.ts';
import { celebration } from '../celebration.svelte.ts';
import { items } from '../data/items.svelte.ts';
import { workflows } from '../data/workflows.svelte.ts';
import { t } from '../i18n/i18n.svelte.ts';
import type { ChipChoice, Mark } from './statechip.ts';

export async function takeChoice(item: WorkItem, choice: Pick<ChipChoice, 'id' | 'label'>): Promise<WorkItem> {
  const moved = choice.id.startsWith('column:')
    ? await items.setBucket(item.id, choice.id.slice('column:'.length), item.version)
    : await workflows.take(item, { step: choice.id.slice('step:'.length) });
  if (moved.completion?.is_completed && !item.completion?.is_completed) void celebration.celebrate(moved);
  if (moved.completion?.is_completed !== item.completion?.is_completed) {
    announcer.say(t(moved.completion?.is_completed ? 'app.entries.completed_announced' : 'app.entries.reopened_announced', { title: item.title }));
  } else {
    announcer.say(t('app.board.moved', { title: item.title, name: choice.label, limit: '' }));
  }
  return moved;
}

/** The mark pressed: completing or reopening by the base product's write, else its step. */
export async function takeMark(item: WorkItem, mark: Mark): Promise<WorkItem> {
  if (mark.kind === 'step') return takeChoice(item, mark.choice);
  const isCompleting = mark.kind === 'complete';
  const answered = await items.setCompleted(item.id, isCompleting, crypto.randomUUID());
  announcer.say(t(isCompleting ? 'app.entries.completed_announced' : 'app.entries.reopened_announced', { title: item.title }));
  if (answered.completion?.is_completed) void celebration.celebrate(answered);
  return answered;
}
