// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import ToastDemo from './_ToastDemo.svelte';

export default {
  title: 'Wave 1 · Feedback/Toast',
  component: ToastDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'zoom'],
} satisfies StoryMeta;

export const tones: Story = {
  name: 'The four tones',
  about:
    'Switch Motion to Reduced and raise one again: the slide becomes an appearance, which is rule 6’s floor. Where the stack sits is the frame’s decision (F1-10) — a toast that positioned itself would put the second one on top of the first.',
};

export const focus: Story = {
  name: 'Announced without stealing focus',
  about:
    'Put the caret in the field, press Save, and watch it stay there. `role="status"` is announced at the next pause; a toast that took focus would throw a keyboard user out of the field they were typing in — which is exactly when a save confirmation arrives.',
  args: { mode: 'focus' },
};
