// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import LogoDemo from './_LogoDemo.svelte';

export default {
  title: 'Wave 5 · Shell/Logo',
  component: LogoDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom'],
} satisfies StoryMeta;

export const lockup: Story = {
  name: 'The mark and the wordmark',
  about:
    'The four size steps with the name beside the mark. The name is passed as it is written — Hubtask — and only the stylesheet lowers it, so copying it or searching for it meets the name. Under RTL the mark stays at the inline start and is not mirrored; the +40 % axis leaves the name alone, because a name is not translated. `sm` draws the reduced mark.',
};

export const steps: Story = {
  name: 'Full and reduced',
  about:
    'Below the use case’s threshold for a legible core the mark is reduced: a heavier outline, no core, a larger spark. The form follows the size step, never a measurement — `sm` is reduced, `md` and up are full. Alone, the mark carries its own accessible name.',
  args: { mode: 'steps' },
};

export const mono: Story = {
  name: 'Mono',
  about:
    'One colour, taken from the text around it: on an accent fill, in print, wherever three colours cannot go. The core is knocked out of the plane by one even-odd path rather than a mask, so two marks on one page share no identifier.',
  args: { mode: 'mono' },
};
