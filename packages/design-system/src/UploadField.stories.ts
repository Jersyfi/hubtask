// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import UploadDemo from './_UploadDemo.svelte';

export default {
  title: 'Wave 3 · Domain/UploadField',
  component: UploadDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'motion', 'density', 'zoom', 'width'],
} satisfies StoryMeta;

export const idle: Story = {
  name: 'Choose a file, by button or by drop',
  about:
    'The native input is transparent and on top, never hidden: hiding it would take the keyboard and the screen reader with it. The drop target is an **addition** — a person who cannot drag can always press, and the button is the path that always exists.',
};

export const chosen: Story = {
  name: 'A file is held, and its size is stated',
  about:
    'The size is shown against the limit as one sentence the caller formed, because a file size is a number with a unit and a unit is a locale (i18n-l10n.md §2). The component moves no bytes: the upload is three steps and all three are the caller’s (arc42 §8.4).',
  args: { mode: 'chosen' },
};

export const uploading: Story = {
  name: 'Bytes on their way, and a way to stop them',
  about:
    'The progress the caller reports, drawn and said. The live region is polite: the reader started this and is not waiting on it the way they wait on a failure, and an assertive one would interrupt them at every percent.',
  args: { mode: 'uploading' },
};

export const over: Story = {
  name: 'Larger than the installation accepts',
  about:
    'Reported, not refused. What an installation accepts is its own answer, and a component that rejected a file locally would reject one the server would have taken. Rule 3 — a sentence, not a red border, and it names the fix.',
  args: { mode: 'over' },
};

export const unavailable: Story = {
  name: 'An installation that stores no attachments',
  about:
    '`disabledReason` is what disables, and it reaches the accessibility tree. `ErrCapabilityNotSupported` must never become silent ignoring: a control that simply did nothing would be indistinguishable from one that is broken.',
  args: { mode: 'unavailable' },
};
