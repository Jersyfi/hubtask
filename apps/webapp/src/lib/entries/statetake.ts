// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * One choice of the state control taken (build-plan §5): a column through the server's `:drop`
 * (completion follows the done column there), a step through the step. The chip's menu, a row's
 * swipe and its menu's *advance* all come here, so the three cannot drift apart.
 */

import type { WorkItem } from '@hubtask/sync-engine';

import { announcer } from '../announce.svelte.ts';
import { celebration } from '../celebration.svelte.ts';
import { items } from '../data/items.svelte.ts';
import { workflows } from '../data/workflows.svelte.ts';
import { t } from '../i18n/i18n.svelte.ts';
import type { ChipChoice } from './statechip.ts';

export async function takeChoice(item: WorkItem, choice: Pick<ChipChoice, 'id' | 'label'>): Promise<WorkItem> {
  const moved = choice.id.startsWith('column:')
    ? (await items.drop(item.id, choice.id.slice('column:'.length), item.version)).item
    : await workflows.take(item, { step: choice.id.slice('step:'.length) });
  if (moved.completion?.is_completed && !item.completion?.is_completed) void celebration.celebrate(moved);
  announcer.say(t('app.board.moved', { title: item.title, name: choice.label, limit: '' }));
  return moved;
}
