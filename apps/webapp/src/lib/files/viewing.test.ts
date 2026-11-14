// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { DRAWN_LIMIT, markOf, neighbour, parseCsv, viewOf, zipListing } from './viewing.ts';

const ready = (content_type: string, file_name: string, extra: { inline?: boolean; size?: number; status?: 'READY' | 'PENDING_SCAN' } = {}) => ({
  content_type, file_name, size: extra.size ?? 1000, status: extra.status ?? 'READY' as const, inline: extra.inline,
});

// F-2: Hubtask shows these itself; F-5: only the server's inline types go to the browser by URL.
test('the viewer shows each kind its own way, and hands only inline types to the browser', () => {
  assert.equal(viewOf(ready('image/png', 'a.png', { inline: true })), 'image');
  assert.equal(viewOf(ready('video/mp4', 'demo.mp4', { inline: true })), 'video');
  assert.equal(viewOf(ready('audio/mpeg', 'talk.mp3', { inline: true })), 'audio');
  assert.equal(viewOf(ready('application/pdf', 'spec.pdf')), 'pdf');
  assert.equal(viewOf(ready('image/svg+xml', 'flow.svg')), 'svg');
  assert.equal(viewOf(ready('text/plain', 'README.md')), 'markdown');
  assert.equal(viewOf(ready('text/plain', 'matrix.csv')), 'csv');
  assert.equal(viewOf(ready('application/zip', 'logs.zip')), 'zip');
  assert.equal(viewOf(ready('text/plain', 'main.go')), 'text');
  assert.equal(viewOf(ready('application/zip', 'Lastenheft.docx')), 'office');
  assert.equal(viewOf(ready('application/octet-stream', 'disk.iso')), 'card');
  // A video the server did not mark inline is never handed over by URL.
  assert.equal(viewOf(ready('video/quicktime', 'clip.mov')), 'card');
  // An image the server did not mark inline (a claim, a type off the list) is not drawn by URL.
  assert.equal(viewOf(ready('image/tiff', 'scan.tif')), 'card');
});

test('a file held for the scanner or too large to draw is a card', () => {
  assert.equal(viewOf(ready('application/pdf', 'held.pdf', { status: 'PENDING_SCAN' })), 'card');
  assert.equal(viewOf(ready('application/pdf', 'huge.pdf', { size: DRAWN_LIMIT + 1 })), 'card');
  // Office files go to the document server whatever their size; the server decides there.
  assert.equal(viewOf(ready('application/zip', 'huge.pptx', { size: DRAWN_LIMIT + 1 })), 'office');
});

test('a mark names the ending, else the type', () => {
  assert.equal(markOf({ content_type: 'application/pdf', file_name: 'x.pdf' }), 'PDF');
  assert.equal(markOf({ content_type: 'application/pdf', file_name: 'noending' }), 'PDF');
});

test('a CSV reads quotes, the separator the first line uses and stops at its limit', () => {
  const { rows } = parseCsv('Schritt;Gerät;Prüfung\n1;"Zweit; gerät";"Bio ""metrie"""\r\n2;Server;Challenge\n');
  assert.deepEqual(rows, [['Schritt', 'Gerät', 'Prüfung'], ['1', 'Zweit; gerät', 'Bio "metrie"'], ['2', 'Server', 'Challenge']]);
  const many = parseCsv(Array.from({ length: 10 }, (_, i) => `r${i},x`).join('\n'), 3);
  assert.equal(many.rows.length, 3);
  assert.equal(many.isCut, true);
});

test('a ZIP is listed from its central directory, and other bytes are no ZIP', () => {
  // A ZIP of two stored entries, built by hand: local headers, central directory, end record.
  const encoder = new TextEncoder();
  const files = [{ name: 'logs/', data: new Uint8Array() }, { name: 'logs/pilot.txt', data: encoder.encode('hello') }];
  const parts: number[] = [];
  const central: number[] = [];
  const u16 = (n: number) => [n & 0xff, (n >> 8) & 0xff];
  const u32 = (n: number) => [n & 0xff, (n >> 8) & 0xff, (n >> 16) & 0xff, (n >> 24) & 0xff];
  for (const file of files) {
    const offset = parts.length;
    const name = [...encoder.encode(file.name)];
    parts.push(...u32(0x04034b50), ...u16(20), ...u16(0), ...u16(0), ...u32(0), ...u32(0), ...u32(file.data.length), ...u32(file.data.length), ...u16(name.length), ...u16(0), ...name, ...file.data);
    central.push(...u32(0x02014b50), ...u16(20), ...u16(20), ...u16(0), ...u16(0), ...u32(0), ...u32(0), ...u32(file.data.length), ...u32(file.data.length),
      ...u16(name.length), ...u16(0), ...u16(0), ...u16(0), ...u16(0), ...u32(0), ...u32(offset), ...name);
  }
  const start = parts.length;
  const bytes = new Uint8Array([...parts, ...central, ...u32(0x06054b50), ...u16(0), ...u16(0), ...u16(2), ...u16(2), ...u32(central.length), ...u32(start), ...u16(0)]);
  const listing = zipListing(bytes);
  assert.deepEqual(listing?.entries, [{ name: 'logs/', size: 0, isFolder: true }, { name: 'logs/pilot.txt', size: 5, isFolder: false }]);
  assert.equal(listing?.total, 2);
  assert.equal(zipListing(encoder.encode('not a zip at all, just words')), undefined);
});

test('← and → wrap at either end', () => {
  assert.equal(neighbour(0, 9, -1), 8);
  assert.equal(neighbour(8, 9, 1), 0);
  assert.equal(neighbour(1, 9, 1), 2);
});
