// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import * as decoding from 'lib0/decoding';
import * as encoding from 'lib0/encoding';
import * as Y from 'yjs';

import { LiveDocument, type LiveSocket } from '../src/live.ts';

/** A socket the test plays the server for: what the client sends is kept, what it gets is pushed. */
class FakeSocket implements LiveSocket {
  binaryType = '';
  readyState = 0;
  onopen: ((event: unknown) => void) | null = null;
  onmessage: ((event: { data: unknown }) => void) | null = null;
  onclose: ((event: { code: number; reason?: string }) => void) | null = null;
  onerror: ((event: unknown) => void) | null = null;
  readonly sent: Uint8Array[] = [];
  send(data: Uint8Array) { this.sent.push(data); }
  close() { this.readyState = 3; }
  open() { this.readyState = 1; this.onopen?.({}); }
  push(message: Uint8Array) { this.onmessage?.({ data: message.buffer.slice(message.byteOffset, message.byteOffset + message.byteLength) }); }
  drop(code = 1006, reason = '') { this.readyState = 3; this.onclose?.({ code, reason }); }
}

/** The server's state vector request (sync step one with an empty vector), as presentation/realtime sends it. */
function serverStep1(): Uint8Array {
  const encoder = encoding.createEncoder();
  encoding.writeVarUint(encoder, 0);
  encoding.writeVarUint(encoder, 0);
  encoding.writeVarUint8Array(encoder, new Uint8Array([0]));
  return encoding.toUint8Array(encoder);
}

/** The updates a client sent, in the order it sent them, as the server stores them. */
function updatesIn(sent: readonly Uint8Array[]): Uint8Array[] {
  const out: Uint8Array[] = [];
  for (const message of sent) {
    const decoder = decoding.createDecoder(message);
    if (decoding.readVarUint(decoder) !== 0) continue;
    const kind = decoding.readVarUint(decoder);
    if (kind === 1 || kind === 2) out.push(decoding.readVarUint8Array(decoder));
  }
  return out;
}

const flush = () => new Promise((resolve) => setImmediate(resolve));

test('two writers at once both keep their words (UC-DOC-02 check 2)', async () => {
  const sockets = [new FakeSocket(), new FakeSocket()];
  const docs = [new Y.Doc(), new Y.Doc()];
  const live = docs.map((doc, at) => new LiveDocument(doc, { open: () => sockets[at]!, writable: true }));
  await flush();
  sockets.forEach((socket) => socket.open());
  docs[0]!.getText('t').insert(0, 'Anna ');
  docs[1]!.getText('t').insert(0, 'Bert ');
  // The server relays each writer's update to the other.
  for (const update of updatesIn(sockets[0]!.sent)) Y.applyUpdate(docs[1]!, update);
  for (const update of updatesIn(sockets[1]!.sent)) Y.applyUpdate(docs[0]!, update);
  assert.equal(docs[0]!.getText('t').toString(), docs[1]!.getText('t').toString());
  assert.match(docs[0]!.getText('t').toString(), /Anna/u);
  assert.match(docs[0]!.getText('t').toString(), /Bert/u);
  live.forEach((each) => each.destroy());
});

test('words typed offline join the page when the device is back (UC-DOC-02 check 3)', async () => {
  const first = new FakeSocket();
  const second = new FakeSocket();
  const opened = [first, second];
  const doc = new Y.Doc();
  let retry: (() => void) | undefined;
  const live = new LiveDocument(doc, { open: () => opened.shift()!, writable: true, schedule: (run) => { retry = run; return () => {}; } });
  await flush();
  first.open();
  first.drop();
  assert.equal(live.status, 'offline');
  doc.getText('t').insert(0, 'typed in the tunnel');
  retry?.();
  await flush();
  second.open();
  // The server asks with its empty state vector; the writer answers with everything it holds.
  second.push(serverStep1());
  const server = new Y.Doc();
  for (const update of updatesIn(second.sent)) Y.applyUpdate(server, update);
  assert.equal(server.getText('t').toString(), 'typed in the tunnel');
  live.destroy();
});

test('a reader sends no update, and a writer answers the server\'s request for a snapshot', async () => {
  const socket = new FakeSocket();
  const doc = new Y.Doc();
  const live = new LiveDocument(doc, { open: () => socket, writable: false });
  await flush();
  socket.open();
  socket.push(serverStep1());
  doc.getText('t').insert(0, 'not sent');
  assert.deepEqual(updatesIn(socket.sent), []);
  live.destroy();

  const writerSocket = new FakeSocket();
  const writer = new LiveDocument(new Y.Doc(), { open: () => writerSocket, writable: true });
  await flush();
  writerSocket.open();
  const due = encoding.createEncoder();
  encoding.writeVarUint(due, 101);
  encoding.writeVarUint(due, 7);
  writerSocket.push(encoding.toUint8Array(due));
  const answer = decoding.createDecoder(writerSocket.sent.at(-1)!);
  assert.equal(decoding.readVarUint(answer), 100);
  assert.equal(decoding.readVarUint(answer), 7);
  writer.destroy();
});

/** The server's opening of a connection: its snapshot as sync step two, then its empty state vector. */
function serverOpening(state: Uint8Array): Uint8Array[] {
  const encoder = encoding.createEncoder();
  encoding.writeVarUint(encoder, 0);
  encoding.writeVarUint(encoder, 1);
  encoding.writeVarUint8Array(encoder, state);
  return [encoding.toUint8Array(encoder), serverStep1()];
}

// SY-F's measurement: writers of a long page reconnecting append what the server lacks, not the
// whole page each - the log grows by the offline words, and a writer with none adds a few bytes.
test('a reconnecting writer sends only what the server lacks (offline-sync.md SY-F)', async () => {
  const page = new Y.Doc();
  const text = page.getText('t');
  for (let at = 0; at < 2_000; at++) text.insert(text.length, `Paragraph ${at} of a long page, written by many hands. `);
  const state = Y.encodeStateAsUpdate(page);
  const writers = 20;
  const server = new Y.Doc();
  Y.applyUpdate(server, state);
  let appended = 0;
  let largest = 0;
  for (let writer = 0; writer < writers; writer++) {
    const doc = new Y.Doc();
    Y.applyUpdate(doc, state);
    const first = new FakeSocket();
    const second = new FakeSocket();
    const opened = [first, second];
    let retry: (() => void) | undefined;
    const live = new LiveDocument(doc, { open: () => opened.shift()!, writable: true, schedule: (run) => { retry = run; return () => {}; } });
    await flush();
    first.open();
    first.drop();
    // Every other writer typed in the tunnel.
    if (writer % 2 === 0) doc.getText('t').insert(0, `w${writer} `);
    retry?.();
    await flush();
    second.open();
    for (const message of serverOpening(Y.encodeStateAsUpdate(server))) second.push(message);
    const sent = updatesIn(second.sent);
    const bytes = sent.reduce((sum, update) => sum + update.byteLength, 0);
    appended += bytes;
    largest = Math.max(largest, bytes);
    for (const update of sent) Y.applyUpdate(server, update);
    assert.equal(server.getText('t').toString().includes(writer % 2 === 0 ? `w${writer} ` : 'Paragraph 1999'), true);
    live.destroy();
  }
  // The whole page was about 110 KB; resending it would append writers × that.
  assert.ok(state.byteLength > 100_000, `the page is ${state.byteLength} bytes`);
  assert.ok(largest < 200, `the largest reconnect sent ${largest} bytes`);
  assert.ok(appended < writers * 200, `the log grew by ${appended} bytes for ${writers} reconnects`);
  for (let writer = 0; writer < writers; writer += 2) assert.match(server.getText('t').toString(), new RegExp(`w${writer} `, 'u'));
});

test('a writer told it writes too fast reconnects no sooner than the server says', async () => {
  const socket = new FakeSocket();
  const waits: number[] = [];
  const live = new LiveDocument(new Y.Doc(), {
    open: () => socket,
    writable: true,
    schedule: (_run, ms) => { waits.push(ms); return () => {}; },
  });
  await flush();
  socket.open();
  socket.drop(4429, 'realtime.too_fast:45');
  assert.deepEqual(waits, [45_000]);
  live.destroy();
});

/** A keeper in memory, as the application's store would hold it across a reload. */
function keeper() {
  let kept: Uint8Array[] = [];
  return {
    get kept() { return kept; },
    load: async () => kept,
    save: async (updates: readonly Uint8Array[]) => { kept = [...updates]; },
  };
}

test('words typed offline are kept, survive a reload, and are sent on the next connection', async () => {
  const store = keeper();
  const first = new FakeSocket();
  const counts: number[] = [];
  const doc = new Y.Doc();
  const live = new LiveDocument(doc, { open: () => first, writable: true, keep: store, onUnsent: (n) => counts.push(n), schedule: () => () => {} });
  await flush();
  first.open();
  first.push(serverStep1());
  first.drop();
  doc.getText('t').insert(0, 'typed on the train');
  await flush();
  assert.equal(live.unsent > 0, true);
  assert.equal(store.kept.length > 0, true, 'the offline words are kept');
  live.destroy();
  doc.destroy();

  // The tab closed and opened again: the kept words are back in the document and go out.
  const second = new FakeSocket();
  const reopened = new Y.Doc();
  const again = new LiveDocument(reopened, { open: () => second, writable: true, keep: store, schedule: () => () => {} });
  await flush();
  assert.equal(reopened.getText('t').toString(), 'typed on the train');
  second.open();
  second.push(serverStep1());
  const server = new Y.Doc();
  for (const update of updatesIn(second.sent)) Y.applyUpdate(server, update);
  assert.equal(server.getText('t').toString(), 'typed on the train');
  await flush();
  assert.equal(again.unsent, 0);
  assert.deepEqual(store.kept, [], 'sent, nothing is kept');
  again.destroy();
});

test('connections that never open are counted, and one that opens clears the count', async () => {
  const sockets = [new FakeSocket(), new FakeSocket(), new FakeSocket(), new FakeSocket()];
  const opened = [...sockets];
  let retry: () => void = () => {};
  const live = new LiveDocument(new Y.Doc(), { open: () => opened.shift()!, writable: true, schedule: (run) => { retry = run; return () => {}; } });
  await new Promise((resolve) => setTimeout(resolve, 0));
  for (const socket of sockets.slice(0, 3)) {
    // A proxy that does not pass the upgrade: the socket closes without ever opening.
    socket.drop(1006);
    retry();
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
  assert.equal(live.failedAttempts, 3);
  sockets[3]!.open();
  assert.equal(live.failedAttempts, 0);
  live.destroy();
});

test('a socket held without an answer is given up at its deadline and counted as failed', async () => {
  const held = new FakeSocket();
  const next = new FakeSocket();
  const opened = [held, next];
  let expire: () => void = () => {};
  let retry: () => void = () => {};
  const live = new LiveDocument(new Y.Doc(), {
    open: () => opened.shift()!, writable: true,
    schedule: (run) => { retry = run; return () => {}; },
    deadline: (run) => { expire = run; return () => {}; },
  });
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(live.status, 'connecting');
  // A proxy that takes the upgrade and never answers it: no open, no close.
  expire();
  assert.equal(held.readyState, 3, 'the held socket is closed');
  assert.equal(live.status, 'offline');
  assert.equal(live.failedAttempts, 1);
  retry();
  await new Promise((resolve) => setTimeout(resolve, 0));
  next.open();
  expire();
  assert.equal(live.status, 'connected', 'the deadline of an opened socket changes nothing');
  live.destroy();
});
