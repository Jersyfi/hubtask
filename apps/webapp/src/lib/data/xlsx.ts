// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * One table as a spreadsheet in the Office Open XML format (ECMA-376), written in the browser the
 * way the server writes the time export: five small parts in a ZIP archive, stored rather than
 * compressed, so no library is carried for it. A text is an inline string, which a spreadsheet
 * never evaluates - no formula prefix is needed, unlike CSV; a number keeps its type.
 */

export const SPREADSHEET_TYPE = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet';

const XML = '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n';
const CONTENT_TYPES = `${XML}<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`;
const ROOT_RELS = `${XML}<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`;
const WORKBOOK_RELS = `${XML}<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`;

/** Escapes a text for an element or an attribute and drops what XML 1.0 cannot carry. */
export function xmlText(value: string): string {
  const kept = value.replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F￾￿]/gu, '');
  return kept.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;').replaceAll("'", '&apos;');
}

/** A column's letters: A … Z, AA … . */
export function columnName(index: number): string {
  let name = '';
  for (let i = index + 1; i > 0; i = Math.floor((i - 1) / 26)) name = String.fromCharCode(65 + ((i - 1) % 26)) + name;
  return name;
}

function sheetXml(heads: readonly string[], rows: readonly (readonly (string | number)[])[]): string {
  const lines = [heads, ...rows].map((cells, r) => {
    const line = r + 1;
    const out = cells.map((cell, c) => {
      const ref = `${columnName(c)}${line}`;
      return typeof cell === 'number' && Number.isFinite(cell)
        ? `<c r="${ref}"><v>${cell}</v></c>`
        : `<c r="${ref}" t="inlineStr"><is><t xml:space="preserve">${xmlText(String(cell))}</t></is></c>`;
    });
    return `<row r="${line}">${out.join('')}</row>`;
  });
  return `${XML}<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>${lines.join('')}</sheetData></worksheet>`;
}

const CRC_TABLE = (() => {
  const table = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    table[n] = c >>> 0;
  }
  return table;
})();

/** The CRC-32 a ZIP entry carries (ISO 3309). */
export function crc32(bytes: Uint8Array): number {
  let crc = 0xffffffff;
  for (const byte of bytes) crc = CRC_TABLE[(crc ^ byte) & 0xff]! ^ (crc >>> 8);
  return (crc ^ 0xffffffff) >>> 0;
}

/** A ZIP archive of stored entries: local headers, the central directory, its end record. */
function stored(parts: readonly { name: string; body: string }[]): Uint8Array {
  const encoder = new TextEncoder();
  const locals: Uint8Array[] = [];
  const centrals: Uint8Array[] = [];
  let offset = 0;
  for (const part of parts) {
    const name = encoder.encode(part.name);
    const body = encoder.encode(part.body);
    const crc = crc32(body);
    const local = new Uint8Array(30 + name.length + body.length);
    const l = new DataView(local.buffer);
    // Version 2.0, no flags, stored, 1980-01-01 00:00.
    l.setUint32(0, 0x04034b50, true); l.setUint16(4, 20, true); l.setUint16(12, 0x21, true);
    l.setUint32(14, crc, true); l.setUint32(18, body.length, true); l.setUint32(22, body.length, true);
    l.setUint16(26, name.length, true);
    local.set(name, 30); local.set(body, 30 + name.length);
    const central = new Uint8Array(46 + name.length);
    const d = new DataView(central.buffer);
    d.setUint32(0, 0x02014b50, true); d.setUint16(4, 20, true); d.setUint16(6, 20, true); d.setUint16(14, 0x21, true);
    d.setUint32(16, crc, true); d.setUint32(20, body.length, true); d.setUint32(24, body.length, true);
    d.setUint16(28, name.length, true); d.setUint32(42, offset, true);
    central.set(name, 46);
    locals.push(local); centrals.push(central);
    offset += local.length;
  }
  const directory = centrals.reduce((sum, each) => sum + each.length, 0);
  const end = new Uint8Array(22);
  const e = new DataView(end.buffer);
  e.setUint32(0, 0x06054b50, true); e.setUint16(8, parts.length, true); e.setUint16(10, parts.length, true);
  e.setUint32(12, directory, true); e.setUint32(16, offset, true);
  const out = new Uint8Array(offset + directory + end.length);
  let at = 0;
  for (const chunk of [...locals, ...centrals, end]) { out.set(chunk, at); at += chunk.length; }
  return out;
}

/** Rows under the given heads as a workbook of one sheet; the sheet's name is a fixed word, never content. */
export function xlsxOf(sheet: string, heads: readonly string[], rows: readonly (readonly (string | number)[])[]): Uint8Array {
  const workbook = `${XML}<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="${xmlText(sheet)}" sheetId="1" r:id="rId1"/></sheets></workbook>`;
  return stored([
    { name: '[Content_Types].xml', body: CONTENT_TYPES },
    { name: '_rels/.rels', body: ROOT_RELS },
    { name: 'xl/workbook.xml', body: workbook },
    { name: 'xl/_rels/workbook.xml.rels', body: WORKBOOK_RELS },
    { name: 'xl/worksheets/sheet1.xml', body: sheetXml(heads, rows) },
  ]);
}
