// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import OneTimeSecretDemo from './_OneTimeSecretDemo.svelte';

export default {
  title: 'Wave 3 · Domain/OneTimeSecret',
  component: OneTimeSecretDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const token: Story = {
  name: 'Shown once',
  about:
    'Hidden until asked for: a secret drawn the moment a panel opens is a secret in whatever screenshot, screen share or shoulder happens to be pointed at it. The mask says nothing about the value — its length included, because a mask as long as the secret narrows a guess for free.',
};

export const acknowledge: Story = {
  name: 'With something to confirm first',
  about:
    'Ten codes scrolled past are ten codes nobody wrote down, so a caller may require a tick before the value can be dismissed. The dismissal is switched off with its reason rather than hidden — `disabledReason` is what disables in this package, so a control the reader cannot use cannot come apart from the why.',
  args: { mode: 'acknowledge' },
};

export const plain: Story = {
  name: 'Nothing to copy, nothing to dismiss',
  about:
    'Copy is offered where the clipboard exists and left out where it does not — an insecure origin has none, and that is not an error: the value is on screen and can be selected. A control that failed at the press would be worse than one that was never there.',
  args: { mode: 'plain' },
};
