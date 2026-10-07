// SPDX-License-Identifier: Apache-2.0
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

export const numbered: Story = {
  name: 'Numbered, with the line the prose is about',
  about:
    'The numbers are in a gutter that is `aria-hidden` and cannot be selected, so a reader who selects the block and copies it gets the code and not “1 2 3 4” down the left — the detail every documentation site gets wrong once. The marked line is the one the paragraph beside it is talking about; it carries a rule down its edge as well as a tint. The file name sits beside the label in the mono face, because it is a path and not a sentence.',
  args: { mode: 'numbered' },
};

export const diff: Story = {
  name: 'What changed, as lines',
  about:
    'Shiki’s transformers without Shiki: an added line, a removed line, and the lines that are still there but are not the point. Every one is a statement about a row of text and none of them needs to know what a keyword is, which is why this works with no grammar and no script. The `+` and the `−` are in the gutter as well as the tint — a diff told apart by colour alone is a diff half the readers cannot read (rule 3), and that is before anybody prints it. The green and the red are the product’s own status surfaces, so they are the same green and red as everywhere else.',
  args: { mode: 'diff' },
};

export const shell: Story = {
  name: 'A shell, whose prompt is not part of the command',
  about:
    'The `$` moves into the gutter: it is punctuation telling the reader “type this”, and a reader who copies it pastes an error. What the copy control hands over is the two commands and neither prompt. It is never inferred from the language — a bash block showing a script has no prompts, and guessing would eat a legitimate `$`.',
  args: { mode: 'shell' },
};

export const wrapped: Story = {
  name: 'Scrolled, and wrapped',
  about:
    'The same long `curl` twice. Scrolling is the default because a wrapped command has lost where its arguments begin; wrapping is there for the payload a reader reads rather than copies. The wrapped remainder hangs under the first column rather than under the gutter, so it reads as a continuation and not as a new line.',
  args: { mode: 'wrapped' },
};

export const withoutCopy: Story = {
  name: 'Without a copy control',
  about:
    'What the website renders: it ships no script (ADR-0030), so it passes no copyLabel and gets no button that could not work. Everything about the lines — the numbers, the marks, the diff, the prompt — survives that, because none of it is a listener. The block is still reachable by keyboard: Tab lands on it, arrows scroll it.',
  args: { mode: 'diff', hasCopy: false },
};

export const long: Story = {
  name: 'A line that overflows',
  about: 'Rule 4 applied to code: the line scrolls, nothing wraps, nothing clips.',
  args: { mode: 'long' },
};
