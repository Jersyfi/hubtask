// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ProgressBarDemo from './_ProgressBarDemo.svelte';

export default {
  title: 'Wave 1 · Feedback/ProgressBar',
  component: ProgressBarDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const determinate: Story = {
  name: 'How far it has got',
  about:
    'The platform’s own `<progress>`, because a bar drawn as a filled div needs a width and a width is arithmetic in an attribute — which the content security policy refuses, in production only. The element takes the number and draws the proportion itself.',
  args: { label: 'Uploading the attachment', value: 37, valueLabel: '37 % uploaded' },
};

export const indeterminate: Story = {
  name: 'Something is happening, and how far is not knowable',
  about:
    'A job answers `progress: null` while it is queued, or while the server cannot yet say how much work there is. A bar sitting at zero would be a claim that nothing has happened; leaving the value off is the honest picture. The track breathes rather than filling — opacity only, so nothing beside it moves, and reduced motion stops it.',
  args: { label: 'Preparing the export' },
};

export const complete: Story = {
  name: 'Finished',
  about:
    'A hundred is not a special case and gets no celebration here: what happens when a thing is done belongs to the screen that started it, not to the bar.',
  args: { label: 'Uploading the attachment', value: 100, valueLabel: 'Uploaded' },
};

export const small: Story = {
  name: 'Beside something rather than under it',
  about:
    'The `sm` size, for a bar inside a row rather than one that owns the space it is in. Density multiplies with it: a compact region gets the tightest bar the tokens allow.',
  args: { label: 'Uploading the attachment', value: 62, valueLabel: '62 % uploaded', size: 'sm' },
};

export const unlabelled: Story = {
  name: 'Without a sentence beside it',
  about:
    'The bar is the whole of it, and the accessible name still says what is progressing. A screen reader reading “37” learns nothing, which is why `valueLabel` exists — but a row that has no room for it is better than a row that invents one.',
  args: { label: 'Uploading the attachment', value: 37 },
};
