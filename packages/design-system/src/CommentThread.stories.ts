// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import CommentDemo from './_CommentDemo.svelte';

export default {
  title: 'Wave 3 · Domain/CommentThread',
  component: CommentDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const thread: Story = {
  name: 'A conversation, oldest first',
  about:
    'One level of replies and no more. A tree of arbitrary depth is one nobody can indent on a phone and nobody can navigate by keyboard without a treeview; one level is what the conversation actually has. Edit and remove are offered **per comment**: on the reader’s own, and on every one for an administrator. On somebody else’s comment they are not drawn at all (`isChangeable: false`) - a person who may never change it is not shown the offer (P-05). Whether this reader is the author is the caller’s knowledge.',
};

export const removed: Story = {
  name: 'A removed comment keeps its place',
  about:
    'The deletion is soft, and the tombstone keeps the author, the time and the position in the order. Hiding the row would leave the reply underneath it dangling, and a reader working out who was answering what. The body is the one thing that is gone — it is not sent, so it is not here to render.',
  args: { mode: 'removed' },
};

export const empty: Story = {
  name: 'Nothing said yet',
  about:
    'voice-and-tone.md §4.1: an empty state names the next step rather than reporting emptiness. A thread with no comments is the normal state of a new entry, not a failure.',
  args: { mode: 'empty' },
};

export const paged: Story = {
  name: 'Older comments, on a control a person presses',
  about:
    '`LoadMore` rather than an infinite scroll: a list that loads on scroll has no end for a keyboard or a screen reader to reach. What arrived is announced in a polite live region, because pressing a button and being told nothing is the case where a sighted reader sees new rows and a screen-reader user hears silence.',
  args: { mode: 'paged' },
};

export const inTableRow: Story = {
  name: 'In a table row',
  about:
    'A comment whose edit and remove are off, beside the same comment with both on: the reasons are bubbles at the buttons, so the two rows are the same height.',
  args: { mode: 'table' },
};

export const notOffered: Story = {
  name: 'Somebody else’s comment offers nothing',
  about:
    'The first comment is the reader’s own, with *Edit* and *Remove*; the second is somebody else’s and this reader is no administrator, so neither control is drawn and no sentence explains their absence (`isChangeable: false`, UC-WRK-19 check 3). A reason is for something the reader could change or wait out; a role that never may is not one.',
  args: { mode: 'others' },
};
