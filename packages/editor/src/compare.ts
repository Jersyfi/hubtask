// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Two versions of a page side by side (UC-DOC-04): their bodies compared line by line, the longest
 * common run kept and the rest marked as taken out or put in. Over Markdown lines, because the
 * version history keeps bodies, not live documents.
 */

export type CompareLine = { readonly kind: 'same' | 'removed' | 'added'; readonly text: string };

/** The most lines compared exactly; beyond it the rest is shown as replaced whole. */
export const MAX_COMPARED_LINES = 4000;

export function compareBodies(before: string, after: string): CompareLine[] {
  const a = before.split('\n');
  const b = after.split('\n');
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
