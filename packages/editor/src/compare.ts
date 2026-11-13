// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { fromMarkdown, toMarkdown } from './markdown.ts';

/**
 * Two versions of a page side by side (UC-DOC-06): their bodies compared line by line, the longest
 * common run kept and the rest marked as taken out or put in. Over Markdown lines, because the
 * version history keeps bodies, not live documents.
 */

export type CompareLine = { readonly kind: 'same' | 'removed' | 'added'; readonly text: string };

/** The most lines compared exactly; beyond it the rest is shown as replaced whole. */
export const MAX_COMPARED_LINES = 4000;

export function compareBodies(before: string, after: string): CompareLine[] {
  return compareSequences(before.split('\n'), after.split('\n'));
}

/** Where two versions differ, block by block, as their texts are drawn (UC-DOC-06/3). */
export interface BlockMarks {
  /** One mark per top-level block of the older version: kept or taken out. */
  readonly before: readonly ('same' | 'removed')[];
  /** One mark per top-level block of the newer version: kept or put in. */
  readonly after: readonly ('same' | 'added')[];
  /** The changed places: runs of blocks taken out or put in that stand together. */
  readonly places: number;
  /** Both versions' blocks in order, each its Markdown, as kept, taken out or put in. */
  readonly blocks: readonly CompareLine[];
}

/**
 * The two bodies compared by their top-level blocks - a paragraph, a heading, a list, a table - so
 * each version can be drawn as formatted text with its changed blocks marked.
 */
export function compareBlocks(before: string, after: string): BlockMarks {
  const blocksOf = (body: string) => (fromMarkdown(body).content ?? []).map((node) => toMarkdown({ type: 'doc', content: [node] }));
  const lines = compareSequences(blocksOf(before), blocksOf(after));
  let places = 0;
  let isInRun = false;
  for (const line of lines) {
    if (line.kind !== 'same' && !isInRun) places++;
    isInRun = line.kind !== 'same';
  }
  return {
    before: lines.filter((line) => line.kind !== 'added').map((line) => line.kind as 'same' | 'removed'),
    after: lines.filter((line) => line.kind !== 'removed').map((line) => line.kind as 'same' | 'added'),
    places,
    blocks: lines,
  };
}

function compareSequences(a: readonly string[], b: readonly string[]): CompareLine[] {
  // The unchanged head and tail are cut off first: most versions differ in a few lines.
  let head = 0;
  while (head < a.length && head < b.length && a[head] === b[head]) head++;
  let tail = 0;
  while (tail < a.length - head && tail < b.length - head && a[a.length - 1 - tail] === b[b.length - 1 - tail]) tail++;
  const middleA = a.slice(head, a.length - tail);
  const middleB = b.slice(head, b.length - tail);
  const out: CompareLine[] = a.slice(0, head).map((text) => ({ kind: 'same', text }));
  if (middleA.length * middleB.length > MAX_COMPARED_LINES * MAX_COMPARED_LINES / 16) {
    out.push(...middleA.map((text) => ({ kind: 'removed' as const, text })), ...middleB.map((text) => ({ kind: 'added' as const, text })));
  } else {
    out.push(...lcs(middleA, middleB));
  }
  out.push(...a.slice(a.length - tail).map((text) => ({ kind: 'same' as const, text })));
  return out;
}

function lcs(a: readonly string[], b: readonly string[]): CompareLine[] {
  const rows = a.length + 1;
  const cols = b.length + 1;
  const table = new Uint32Array(rows * cols);
  for (let i = a.length - 1; i >= 0; i--) {
    for (let j = b.length - 1; j >= 0; j--) {
      table[i * cols + j] = a[i] === b[j]
        ? (table[(i + 1) * cols + j + 1] ?? 0) + 1
        : Math.max(table[(i + 1) * cols + j] ?? 0, table[i * cols + j + 1] ?? 0);
    }
  }
  const out: CompareLine[] = [];
  let i = 0;
  let j = 0;
  while (i < a.length && j < b.length) {
    if (a[i] === b[j]) {
      out.push({ kind: 'same', text: a[i] ?? '' });
      i++;
      j++;
    } else if ((table[(i + 1) * cols + j] ?? 0) >= (table[i * cols + j + 1] ?? 0)) {
      out.push({ kind: 'removed', text: a[i++] ?? '' });
    } else {
      out.push({ kind: 'added', text: b[j++] ?? '' });
    }
  }
  while (i < a.length) out.push({ kind: 'removed', text: a[i++] ?? '' });
  while (j < b.length) out.push({ kind: 'added', text: b[j++] ?? '' });
  return out;
}
