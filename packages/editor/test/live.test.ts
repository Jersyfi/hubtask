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
  onclose: ((event: { code: number }) => void) | null = null;
  onerror: ((event: unknown) => void) | null = null;
  readonly sent: Uint8Array[] = [];
  send(data: Uint8Array) { this.sent.push(data); }
  close() { this.readyState = 3; }
  open() { this.readyState = 1; this.onopen?.({}); }
  push(message: Uint8Array) { this.onmessage?.({ data: message.buffer.slice(message.byteOffset, message.byteOffset + message.byteLength) }); }
  drop(code = 1006) { this.readyState = 3; this.onclose?.({ code }); }
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
