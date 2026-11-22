// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A new board is made at once and opened (scene wnew, C-3): blank, it opens with its templates as
// a sheet; from a template card, it opens with the template on it and the template's name.

import { elementId } from '@hubtask/whiteboard';

import { newBoardPath, whiteboards } from '../data/whiteboards.svelte.ts';
import { defaultName, t } from '../i18n/i18n.svelte.ts';
import { startElements, templateOf, type BoardStart } from './BoardTemplates.svelte';

/** Makes a board in the hub and answers where it opens. */
export async function makeBoard(hubId: string, choice: BoardStart = 'BLANK', parentPageId?: string): Promise<string> {
  const name = templateOf(choice);
  const elements = name ? startElements(name, (key) => t(`app.whiteboards.word_${key}`), elementId) : [];
  const made = await whiteboards.create(hubId, {
    // A stored name, so in the account's language, else the workspace's; a template's choice
    // renames a blank board while it still has these words (WhiteboardsView).
    title: await defaultName(name ? `app.whiteboards.start_${choice.toLowerCase()}` : 'app.create.untitled', name ? undefined : { kind: 'whiteboard' }),
    ...(parentPageId ? { parent_page_id: parentPageId } : {}),
    ...(elements.length > 0 ? { elements: elements as never } : {}),
  });
  return name ? `/whiteboards/${made.id}` : newBoardPath(made.id);
}
