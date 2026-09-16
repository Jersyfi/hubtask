// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { Story, StoryMeta } from '../workbench/lib/story.ts';
import CodeBlockDemo from './_CodeBlockDemo.svelte';

export default {
  title: 'Wave 4 · Documentation/CodeBlock',
  component: CodeBlockDemo,
  status: 'draft',
  axes: ['theme', 'dir', 'text', 'zoom', 'width'],
} satisfies StoryMeta;

export const request: Story = {
  name: 'A request and its response',
  about:
    'Mono, no highlighter, a language label, and a copy control where the caller asked for one. The block keeps its own scroll: pull Width down and the long line scrolls inside the box rather than widening the page. Under dir=rtl the code still runs left to right, because code does.',
};

export const withoutCopy: Story = {
  name: 'Without a copy control',
  about:
    'What the website renders: it ships no script (ADR-0030), so it passes no copyLabel and gets no button that could not work. The block is still reachable by keyboard - Tab lands on it, arrows scroll it.',
  args: { hasCopy: false },
};

export const long: Story = {
  name: 'A line that overflows',
  about: 'Rule 4 applied to code: the line scrolls, nothing wraps, nothing clips.',
  args: { isLong: true },
};
