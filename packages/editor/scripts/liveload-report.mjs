// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What scenario S5 (scripts/liveload.mjs) judges its writers by: whether their documents are one
// document, and the figures it writes for the load run.

import * as Y from 'yjs';

/** Whether every document holds the same state: equal state vectors and equal content. */
export function converged(docs) {
  if (docs.length === 0) return true;
  const vector = (doc) => Buffer.from(Y.encodeStateVector(doc)).toString('base64');
  const first = docs[0];
  return docs.every(
    (doc) => vector(doc) === vector(first) && doc.getText('load').toString() === first.getText('load').toString(),
  );
}

/** The run's figures; `equal` is the bar, the rest is recorded. */
export function summarise({ writers, docs, fresh, windowMs, settled, awarenessSeen, closes, closedAt = [] }) {
  const sum = (key) => writers.reduce((n, w) => n + w[key], 0);
  return {
    test: 'S5 live documents with many writers',
    writers: writers.length,
    window_ms: windowMs,
    edits: sum('edits'),
    cursors: sum('cursors'),
    unsent: sum('unsent'),
    awareness_seen: awarenessSeen,
    equal: settled === true && converged(docs),
    fresh_equal: converged([...docs, fresh]),
    length: docs[0]?.getText('load').length ?? 0,
    closes_by_code: closes,
    first_closes: closedAt,
  };
}
