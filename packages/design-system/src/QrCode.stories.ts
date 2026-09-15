// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import QrCodeDemo from './_QrCodeDemo.svelte';

export default {
  title: 'Wave 3 · Domain/QrCode',
  component: QrCodeDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const provisioning: Story = {
  name: 'A provisioning URI',
  about:
    'The one input this exists for: an `otpauth://` URI, drawn beside the same secret written out (ADR-0053). Dark on light in both modes, because a camera is not a reader of the theme and not every authenticator reads an inverted code.',
};

export const smallest: Story = {
  name: 'The smallest symbol',
  about: 'Version 1, twenty-one modules a side: what a one-byte payload produces.',
  args: { mode: 'smallest' },
};

export const largest: Story = {
  name: 'The largest this encoder draws',
  about:
    'Version 13, sixty-nine modules a side, 331 bytes - past which `encode` refuses by name rather than guessing at tables it does not carry, and the caller shows the secret without an image.',
  args: { mode: 'largest' },
};
