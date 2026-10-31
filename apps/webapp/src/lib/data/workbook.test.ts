// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A workbook's columns read in the browser for an asset import's mapping (UC-AST-04 check 6): the
// header row of its first sheet, by the rules the server reads the whole sheet with.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { base64Of, isZip, workbookColumns } from './workbook.ts';

const WORKBOOK = `<?xml version="1.0" encoding="UTF-8"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Assets" sheetId="1" r:id="rId7"/><sheet name="Other" sheetId="2" r:id="rId8"/></sheets></workbook>`;
const RELS = `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId8" Target="worksheets/sheet2.xml"/><Relationship Id="rId7" Target="worksheets/sheet1.xml"/></Relationships>`;
const SHARED = `<sst><si><t>name</t></si><si><t xml:space="preserve"> serial </t></si><si><r><t>Purchase </t></r><r><t>&amp; price</t></r></si></sst>`;
const SHEET = `<worksheet><sheetData>
<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="D1" t="s"><v>2</v></c><c r="E1" t="inlineStr"><is><t>active</t></is></c></row>
<row r="2"><c r="A2" t="s"><v>0</v></c></row>
</sheetData></worksheet>`;
const OTHER = `<worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>wrong sheet</t></is></c></row></sheetData></worksheet>`;

/** A ZIP archive of the parts, each deflated or stored. */
async function zip(parts: Record<string, string>, deflate: boolean): Promise<Uint8Array> {
  const encoder = new TextEncoder();
  const locals: Uint8Array[] = [];
  const centrals: Uint8Array[] = [];
  let offset = 0;
  for (const [name, text] of Object.entries(parts)) {
    const nameBytes = encoder.encode(name);
    const raw = encoder.encode(text);
    const body = deflate
      ? new Uint8Array(await new Response(new Blob([raw]).stream().pipeThrough(new CompressionStream('deflate-raw'))).arrayBuffer())
      : raw;
    const local = new Uint8Array(30 + nameBytes.length);
    const lv = new DataView(local.buffer);
    lv.setUint32(0, 0x04034b50, true);
    lv.setUint16(8, deflate ? 8 : 0, true);
    lv.setUint32(18, body.length, true);
    lv.setUint32(22, raw.length, true);
    lv.setUint16(26, nameBytes.length, true);
    local.set(nameBytes, 30);
    const central = new Uint8Array(46 + nameBytes.length);
    const cv = new DataView(central.buffer);
    cv.setUint32(0, 0x02014b50, true);
    cv.setUint16(10, deflate ? 8 : 0, true);
    cv.setUint32(20, body.length, true);
    cv.setUint32(24, raw.length, true);
    cv.setUint16(28, nameBytes.length, true);
    cv.setUint32(42, offset, true);
    central.set(nameBytes, 46);
    locals.push(local, body);
    centrals.push(central);
    offset += local.length + body.length;
  }
  const size = centrals.reduce((sum, each) => sum + each.length, 0);
  const end = new Uint8Array(22);
  const ev = new DataView(end.buffer);
  ev.setUint32(0, 0x06054b50, true);
  ev.setUint16(8, centrals.length, true);
  ev.setUint16(10, centrals.length, true);
  ev.setUint32(12, size, true);
  ev.setUint32(16, offset, true);
  const all = [...locals, ...centrals, end];
  const out = new Uint8Array(all.reduce((sum, each) => sum + each.length, 0));
  let at = 0;
  for (const each of all) { out.set(each, at); at += each.length; }
  return out;
}

const PARTS = {
  'xl/workbook.xml': WORKBOOK,
  'xl/_rels/workbook.xml.rels': RELS,
  'xl/sharedStrings.xml': SHARED,
  'xl/worksheets/sheet1.xml': SHEET,
  'xl/worksheets/sheet2.xml': OTHER,
};

test('the first sheet\'s header row is the columns, by its relationship, trimmed and without empty cells', async () => {
  for (const deflate of [true, false]) {
    const bytes = await zip(PARTS, deflate);
    assert.ok(isZip(bytes));
    assert.deepEqual(await workbookColumns(bytes), ['name', 'serial', 'Purchase & price', 'active']);
  }
});

test('a file that is no workbook answers no columns, for the server to refuse', async () => {
  assert.deepEqual(await workbookColumns(new TextEncoder().encode('name,serial\n')), []);
  assert.deepEqual(await workbookColumns(await zip({ 'word/document.xml': '<w/>' }, true)), []);
});

test('a sheet whose first row is not row 1 has no header', async () => {
  const late = `<worksheet><sheetData><row r="2"><c r="A2" t="inlineStr"><is><t>name</t></is></c></row></sheetData></worksheet>`;
  assert.deepEqual(await workbookColumns(await zip({ ...PARTS, 'xl/worksheets/sheet1.xml': late }, true)), []);
});

test('a workbook travels as base64', () => {
  assert.equal(base64Of(new Uint8Array([0x50, 0x4b, 0x03, 0x04])), 'UEsDBA==');
});
