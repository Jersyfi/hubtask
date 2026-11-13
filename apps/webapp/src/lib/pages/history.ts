// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Two versions of a page as the history shows them (UC-DOC-06/3): side by side, the older one's
 * lines with what went out, the page's with what came in; or only the changed places, each a run
 * of lines taken out and put in that stand together.
 */

import type { CompareLine } from '@hubtask/editor';

export function sideBySide(lines: readonly CompareLine[]): { before: CompareLine[]; after: CompareLine[] } {
  return { before: lines.filter((line) => line.kind !== 'added'), after: lines.filter((line) => line.kind !== 'removed') };
}

export function changeRuns(lines: readonly CompareLine[]): CompareLine[][] {
  const runs: CompareLine[][] = [];
  let run: CompareLine[] = [];
  for (const line of lines) {
    if (line.kind === 'same') {
      if (run.length > 0) runs.push(run);
      run = [];
    } else {
      run.push(line);
    }
  }
  if (run.length > 0) runs.push(run);
  return runs;
}
