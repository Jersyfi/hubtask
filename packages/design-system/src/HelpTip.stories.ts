// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import HelpTipDemo from './_HelpTipDemo.svelte';

export default {
  title: 'Wave 1 · Overlays/HelpTip',
  component: HelpTipDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom', 'width'],
} satisfies StoryMeta;

export const label: Story = {
  name: 'Beside a field’s label',
  about:
    'The hint under the field says what is needed now; the ⓘ beside the label holds the why. A click, a tap, Enter or Space opens it — a hover never does — and its words take focus so a screen reader reads them. Escape or a press elsewhere closes it, and focus goes back to the ⓘ, named “More about Expires”.',
};

export const heading: Story = {
  name: 'Beside a heading and a setting’s name',
  about:
    'A section keeps its one sentence under the heading and puts the rest behind the ⓘ beside it; a setting keeps the line that says what it does and puts the why beside its name. The ⓘ is beside the words, never inside them, so it is not part of the heading’s or the setting’s name.',
  args: { mode: 'heading' },
};

export const long: Story = {
  name: 'Three sentences, and the end of the line',
  about:
    'Three sentences is the most it holds: the popover wraps them at a reading width and never cuts them. Open the one at the end of the line: the popover turns to the side with room rather than running off the screen.',
  args: { mode: 'long' },
};

export const rtl: Story = {
  name: 'Right to left',
  about:
    'In a right-to-left box the ⓘ follows the label at its start, and the popover opens from it towards the line’s end in that direction.',
  args: { mode: 'rtl' },
};

export const phone: Story = {
  name: 'On a phone',
  about:
    'Set the width to compact or the window to a phone and tap the ⓘ: it stays a popover at the ⓘ, not a sheet — three sentences do not need one, and a sheet would cover the field they explain. A tap beside it closes it.',
  args: { mode: 'phone' },
};
