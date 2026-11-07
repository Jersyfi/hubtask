// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Scenario S5 of test/load/PLAN.md: many writers on one live document through the editors' own
// connection (src/live.ts), each sending an edit once a second and its cursor ten times a second.
// At the end every writer's document, and one opened afresh, must be the same. The load run starts
// it (test/load/live_test.go); by hand:
//
//   node scripts/liveload.mjs <ws url of the document> <out.json> <seconds> <bearer>...
//
// The writers are spread over every bearer but the last in turn, so no credential holds more than
// the change stream's per-credential cap; the last is the fresh opener's.

import { writeFileSync } from 'node:fs';
import * as Y from 'yjs';
import { LiveDocument } from '../src/live.ts';
import { converged, summarise } from './liveload-report.mjs';

const WRITERS = 30;
const EDIT_EVERY_MS = 1_000;
const CURSOR_EVERY_MS = 100;
// How long the writers get after the window to reach one another, and the fresh opener to sync.
const SETTLE_MS = 30_000;

const [url, out, seconds, ...bearers] = process.argv.slice(2);
if (!url || !out || !Number(seconds) || bearers.length === 0) {
  console.error('usage: node scripts/liveload.mjs <ws url> <out.json> <seconds> <bearer>...');
  process.exit(2);
}

const closes = {};
/** When each abnormal close came, in ms from the start, the first few: a diagnosis, not a bar. */
const closedAt = [];
const begun = Date.now();
/**
 * Opens a socket as the browser does (the sync engine's `liveSocket`): the protocol's subprotocol
 * and the bearer as a second one, binary as ArrayBuffer.
 */
function opener(bearer) {
  return () => {
    const socket = new WebSocket(url, ['hubtask.v1', `hubtask.bearer.${bearer}`]);
    socket.binaryType = 'arraybuffer';
    socket.addEventListener('close', (event) => {
      if (event.code === 1000) return;
      closes[event.code] = (closes[event.code] ?? 0) + 1;
      if (closedAt.length < 10) closedAt.push({ code: event.code, reason: event.reason, at_ms: Date.now() - begun });
    });
    return socket;
  };
}

function writer(bearer) {
  const doc = new Y.Doc();
  const live = new LiveDocument(doc, { open: opener(bearer), writable: true });
  return { doc, live, edits: 0, cursors: 0 };
}

const until = (test, ms) =>
  new Promise((resolve) => {
    const deadline = Date.now() + ms;
    const poll = () => (test() || Date.now() > deadline ? resolve(test()) : setTimeout(poll, 100));
    poll();
  });

const writing = bearers.length > 1 ? bearers.slice(0, -1) : bearers;
const writers = Array.from({ length: WRITERS }, (_, i) => writer(writing[i % writing.length]));
let awarenessSeen = 0;
for (const w of writers) w.live.awareness.on('change', (_, origin) => origin !== 'local' && (awarenessSeen += 1));

const started = Date.now();
const timers = [];
writers.forEach((w, i) => {
  timers.push(
    setInterval(() => {
      w.doc.getText('load').insert(0, `w${i} e${w.edits} `);
      w.edits += 1;
    }, EDIT_EVERY_MS),
    setInterval(() => {
      w.live.awareness.setLocalStateField('cursor', { at: w.cursors % 1000 });
      w.cursors += 1;
    }, CURSOR_EVERY_MS),
  );
});
await new Promise((resolve) => setTimeout(resolve, Number(seconds) * 1000));
for (const timer of timers) clearInterval(timer);
const windowMs = Date.now() - started;

const docs = writers.map((w) => w.doc);
const settled = await until(() => converged(docs), SETTLE_MS);
const fresh = writer(bearers[bearers.length - 1]);
await until(() => converged([...docs, fresh.doc]), SETTLE_MS);

const report = summarise({
  writers: writers.map((w) => ({ edits: w.edits, cursors: w.cursors, unsent: w.live.unsent })),
  docs,
  fresh: fresh.doc,
  windowMs,
  settled,
  awarenessSeen,
  closes,
  closedAt,
});
for (const w of [...writers, fresh]) w.live.destroy();
writeFileSync(out, JSON.stringify(report, null, 2) + '\n');
console.log(
  `S5: ${report.writers} writers, ${report.edits} edits, ${report.cursors} cursors over ${windowMs} ms; ` +
    `equal ${report.equal}, fresh opener equal ${report.fresh_equal}, unsent ${report.unsent}, closes ${JSON.stringify(closes)}`,
);
process.exit(0);
