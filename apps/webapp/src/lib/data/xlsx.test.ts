// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { columnName, crc32, xlsxOf, xmlText } from './xlsx.ts';

/** Reads a stored ZIP back through its central directory, as a spreadsheet opens it. */
function entries(archive: Uint8Array): Map<string, string> {
  const view = new DataView(archive.buffer, archive.byteOffset, archive.byteLength);
  const end = archive.length - 22;
  assert.equal(view.getUint32(end, true), 0x06054b50);
  const count = view.getUint16(end + 10, true);
  let at = view.getUint32(end + 16, true);
  const out = new Map<string, string>();
  for (let i = 0; i < count; i++) {
    assert.equal(view.getUint32(at, true), 0x02014b50);
    const crc = view.getUint32(at + 16, true);
    const size = view.getUint32(at + 24, true);
    const nameLength = view.getUint16(at + 28, true);
    const local = view.getUint32(at + 42, true);
    const name = new TextDecoder().decode(archive.subarray(at + 46, at + 46 + nameLength));
    const start = local + 30 + view.getUint16(local + 26, true);
    const body = archive.subarray(start, start + size);
    assert.equal(crc32(body), crc, name);
    out.set(name, new TextDecoder().decode(body));
    at += 46 + nameLength;
  }
  return out;
}

test('the CRC is the one every ZIP reader checks', () => {
  assert.equal(crc32(new TextEncoder().encode('123456789')), 0xcbf43926);
});

test('columns are lettered as a spreadsheet letters them', () => {
  assert.deepEqual([0, 25, 26, 27, 701, 702].map(columnName), ['A', 'Z', 'AA', 'AB', 'ZZ', 'AAA']);
});

// UC-GOL-04 check 13: a widget's table is handed over as a spreadsheet, a number staying a number
// and a formula never running.
test('a table becomes a workbook of one sheet', () => {
  const parts = entries(xlsxOf('Widget', ['Group', 'Value'], [['=SUM(A1)', 2], ['a < b & "c"', 3.5]]));
  assert.deepEqual([...parts.keys()], ['[Content_Types].xml', '_rels/.rels', 'xl/workbook.xml', 'xl/_rels/workbook.xml.rels', 'xl/worksheets/sheet1.xml']);
  const sheet = parts.get('xl/worksheets/sheet1.xml') ?? '';
  assert.match(sheet, /<c r="A2" t="inlineStr"><is><t xml:space="preserve">=SUM\(A1\)<\/t><\/is><\/c><c r="B2"><v>2<\/v><\/c>/u);
  assert.match(sheet, /a &lt; b &amp; &quot;c&quot;/u);
  assert.match(sheet, /<c r="B3"><v>3.5<\/v><\/c>/u);
  assert.match(parts.get('xl/workbook.xml') ?? '', /<sheet name="Widget"/u);
});

test('what XML cannot carry is dropped', () => {
  assert.equal(xmlText('a\u0001b\tc'), 'ab\tc');
});
