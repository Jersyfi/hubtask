// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { TransportError } from '@hubtask/sync-engine';

import { assertZipComplete, zipComplete } from './zipComplete.ts';

/** The end record of an archive without a comment: the signature and eighteen bytes. */
const END = [0x50, 0x4b, 0x05, 0x06, ...new Array<number>(18).fill(0)];

function target() {
  const bytes: number[] = [];
  const ended: string[] = [];
  const sink = new WritableStream<Uint8Array>({
    write: (chunk) => { bytes.push(...chunk); },
    close: () => { ended.push('closed'); },
    abort: () => { ended.push('aborted'); },
  });
  return { sink, bytes, ended };
}

async function feed(sink: WritableStream<Uint8Array>, chunks: number[][]): Promise<void> {
  await new ReadableStream<Uint8Array>({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(new Uint8Array(chunk));
      controller.close();
    },
  }).pipeTo(sink);
}

test('an archive that ends with its end record is written through and kept', async () => {
  const { sink, bytes, ended } = target();
  // The end record split across two chunks, as a network hands it over.
  await feed(zipComplete(sink), [[1, 2, 3, ...END.slice(0, 5)], END.slice(5)]);
  assert.equal(bytes.length, 3 + END.length);
  // A chunk longer than the record carries it whole.
  const second = target();
  await feed(zipComplete(second.sink), [[...new Array<number>(30).fill(7), ...END]]);
  assert.deepEqual(second.ended, ['closed']);
  assert.deepEqual(ended, ['closed']);
});

// The server ends a broken-off archive without its end record and the response still ends cleanly:
// keeping that file would hand the owner a backup that does not open.
test('an archive cut short is discarded rather than kept', async () => {
  const { sink, ended } = target();
  await assert.rejects(() => feed(zipComplete(sink), [[0x50, 0x4b, 0x03, 0x04, 9, 9, 9]]), (error: unknown) => error instanceof TransportError && error.detailCode === 'app.backup.download_incomplete');
  assert.deepEqual(ended, ['aborted']);
});

test('an archive whose stream breaks off discards its target', async () => {
  const { sink, ended } = target();
  const guarded = zipComplete(sink);
  const writer = guarded.getWriter();
  await writer.write(new Uint8Array([1]));
  await writer.abort(new Error('network'));
  assert.deepEqual(ended, ['aborted']);
});

test('a file held whole is handed over only when it ends with the end record', async () => {
  await assertZipComplete(new Blob([new Uint8Array([1, 2, ...END])]));
  await assert.rejects(
    () => assertZipComplete(new Blob([new Uint8Array([0x50, 0x4b, 0x03, 0x04])])),
    (error: unknown) => error instanceof TransportError && error.detailCode === 'app.backup.download_incomplete',
  );
});
