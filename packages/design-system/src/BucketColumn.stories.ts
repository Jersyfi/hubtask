// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import BoardDemo from './_BoardDemo.svelte';

export default {
  title: 'Wave 3 · Domain/BucketColumn',
  component: BoardDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const board: Story = {
  name: 'Three columns, one of them the done bucket',
  about:
    'A column is a **region** with an accessible name and its cards are inside it — a board of divs is one long run of titles a screen reader cannot tell apart. `isDoneBucket` is announced here and acted on by the board, which is the domain’s own division: Bucket.IsDoneBucket is “stored and reported; what reacts to it is the client that renders the board … the server completes nothing on its own account”. A component that completed an entry would be a component with a write in it.',
};

export const overLimit: Story = {
  name: 'Over its WIP limit',
  about:
    'Said, **not enforced**. The server accepts a card that takes a column past its `wipLimit` — the limit is a property of the bucket, not a constraint on the write — so a client that blocked the drop would be inventing a rule the workspace does not have. The column says it is over and the reader decides. Rule 3: the mark and the sentence carry it, not the colour alone.',
  args: { mode: 'overLimit' },
};

export const long: Story = {
  name: 'The same column in German',
  about:
    'Rule 4. The column keeps its width and the card wraps; the board scrolls sideways rather than the page, because a wide board that widened the document would take every other region with it.',
  args: { mode: 'long' },
};
