// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The changed places of two page versions (UC-DOC-06/3), as the history's *changes only* shows them:
 * each a run of blocks taken out and put in that stand together.
 */

import type { CompareLine } from '@hubtask/editor';

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
