// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The column names of a workbook's first sheet, read in the browser for an import's mapping
 * (UC-AST-04 check 6). Only the header row is read here; the server reads the whole sheet with the
 * same rules (`core/shared/spreadsheet`): the workbook's first sheet through its relationships,
 * shared and inline strings, rich-text runs joined.
 *
 * The platform's own `DecompressionStream` inflates the parts, so no library is needed. A file
 * that is not a workbook answers no columns, and the server's refusal says why.
 */

/** A part the header needs is bounded, as the server bounds every part it reads. */
const MAX_PART = 8 * 1024 * 1024;

interface Entry {
  readonly method: number;
  readonly compressed: number;
  readonly offset: number;
}

/** Whether the bytes start as a ZIP archive does, as an XLSX workbook is one. */
export const isZip = (bytes: Uint8Array): boolean => bytes.length >= 4 && bytes[0] === 0x50 && bytes[1] === 0x4b && bytes[2] === 0x03 && bytes[3] === 0x04;

/** The archive's entries by name, from its central directory. */
function entries(bytes: Uint8Array): Map<string, Entry> | undefined {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  // The end of central directory record is in the last 64 KiB plus its own 22 bytes.
  let end = -1;
  for (let at = bytes.length - 22; at >= Math.max(0, bytes.length - 22 - 65535); at--) {
    if (view.getUint32(at, true) === 0x06054b50) { end = at; break; }
  }
  if (end < 0) return undefined;
  const count = view.getUint16(end + 10, true);
  let at = view.getUint32(end + 16, true);
  const out = new Map<string, Entry>();
  const names = new TextDecoder();
  for (let i = 0; i < count; i++) {
    if (at + 46 > bytes.length || view.getUint32(at, true) !== 0x02014b50) return undefined;
    const method = view.getUint16(at + 10, true);
    const compressed = view.getUint32(at + 20, true);
    const nameLength = view.getUint16(at + 28, true);
    const extraLength = view.getUint16(at + 30, true);
    const commentLength = view.getUint16(at + 32, true);
    const offset = view.getUint32(at + 42, true);
    out.set(names.decode(bytes.subarray(at + 46, at + 46 + nameLength)), { method, compressed, offset });
    at += 46 + nameLength + extraLength + commentLength;
  }
  return out;
}

/** One part's text, inflated where it is deflated; undefined where it is missing or unreadable. */
async function part(bytes: Uint8Array, all: Map<string, Entry>, name: string): Promise<string | undefined> {
  const entry = all.get(name);
  if (!entry || entry.compressed > MAX_PART) return undefined;
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const local = entry.offset;
  if (local + 30 > bytes.length || view.getUint32(local, true) !== 0x04034b50) return undefined;
  const start = local + 30 + view.getUint16(local + 26, true) + view.getUint16(local + 28, true);
  const body = bytes.slice(start, start + entry.compressed);
  if (entry.method === 0) return new TextDecoder().decode(body);
  if (entry.method !== 8) return undefined;
  try {
    const stream = new Blob([body]).stream().pipeThrough(new DecompressionStream('deflate-raw'));
    const inflated = await new Response(stream).arrayBuffer();
    return inflated.byteLength > MAX_PART ? undefined : new TextDecoder().decode(inflated);
  } catch {
    return undefined;
  }
}

const ENTITIES: Readonly<Record<string, string>> = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'" };

/** XML text with its entities resolved. */
function unescape(text: string): string {
  return text.replace(/&(#x[0-9a-f]+|#[0-9]+|[a-z]+);/giu, (whole, name: string) => {
    if (name.startsWith('#x') || name.startsWith('#X')) return String.fromCodePoint(Number.parseInt(name.slice(2), 16));
    if (name.startsWith('#')) return String.fromCodePoint(Number.parseInt(name.slice(1), 10));
    return ENTITIES[name] ?? whole;
  });
}

/** A string's text: its `<t>` runs joined, as a rich-text string holds them. */
const runs = (xml: string): string => [...xml.matchAll(/<t(?:\s[^>]*)?>([\s\S]*?)<\/t>/gu)].map((match) => unescape(match[1] ?? '')).join('');

/** An attribute's value on a tag. */
const attribute = (tag: string, name: string): string | undefined => tag.match(new RegExp(`(?:^|\\s)${name}="([^"]*)"`, 'u'))?.[1];

/** The part the workbook's first sheet is in. */
function firstSheetPart(workbook: string, rels: string | undefined): string {
  const sheet = workbook.match(/<(?:\w+:)?sheet\s[^>]*>/u)?.[0];
  const id = sheet ? attribute(sheet, 'r:id') ?? attribute(sheet, '\\w+:id') : undefined;
  for (const tag of rels?.match(/<(?:\w+:)?Relationship\s[^>]*>/gu) ?? []) {
    if (attribute(tag, 'Id') !== id) continue;
    const target = attribute(tag, 'Target') ?? '';
    const joined = target.startsWith('/') ? target.slice(1) : `xl/${target}`;
    // `path.Clean` as the server does it: `.` and `..` steps resolved.
    const steps: string[] = [];
    for (const step of joined.split('/')) {
      if (step === '..') steps.pop();
      else if (step !== '.' && step !== '') steps.push(step);
    }
    return steps.join('/');
  }
  return 'xl/worksheets/sheet1.xml';
}

/** A cell reference's column, from 0: `A1` is 0, `AB7` is 27. */
function columnOf(ref: string): number | undefined {
  const letters = ref.match(/^[A-Z]+/u)?.[0];
  if (!letters) return undefined;
  let column = 0;
  for (const letter of letters) column = column * 26 + (letter.charCodeAt(0) - 64);
  return column - 1;
}

/**
 * The first sheet's header row as column names, trimmed and without empty ones - what a CSV
 * table's first line answers - or none where the bytes are no workbook.
 */
export async function workbookColumns(bytes: Uint8Array): Promise<string[]> {
  if (!isZip(bytes)) return [];
  const all = entries(bytes);
  if (!all) return [];
  const workbook = await part(bytes, all, 'xl/workbook.xml');
  if (!workbook) return [];
  const sheet = await part(bytes, all, firstSheetPart(workbook, await part(bytes, all, 'xl/_rels/workbook.xml.rels')));
  if (!sheet) return [];
  const shared = [...((await part(bytes, all, 'xl/sharedStrings.xml')) ?? '').matchAll(/<si>([\s\S]*?)<\/si>/gu)].map((match) => runs(match[1] ?? ''));

  const row = sheet.match(/<row(?:\s[^>]*)?>([\s\S]*?)<\/row>/u);
  if (!row) return [];
  const rowTag = row[0].match(/^<row(?:\s[^>]*)?>/u)?.[0] ?? '';
  if (rowTag.endsWith('/>')) return [];
  // A sheet whose first row is not row 1 has an empty header, as the server reads it.
  const number = attribute(rowTag, 'r');
  if (number !== undefined && number !== '1') return [];
  const cells: string[] = [];
  for (const cell of (row[1] ?? '').matchAll(/<c(\s[^>]*)?(?:\/>|>([\s\S]*?)<\/c>)/gu)) {
    const tag = cell[1] ?? '';
    const inner = cell[2] ?? '';
    const ref = attribute(tag, 'r');
    const column = ref ? columnOf(ref) : cells.length;
    if (column === undefined || column < cells.length) return [];
    while (cells.length < column) cells.push('');
    const value = unescape(inner.match(/<v>([\s\S]*?)<\/v>/u)?.[1] ?? '');
    const kind = attribute(tag, 't');
    if (kind === 's') cells.push(shared[Number.parseInt(value.trim(), 10)] ?? '');
    else if (kind === 'inlineStr') cells.push(runs(inner.match(/<is>([\s\S]*?)<\/is>/u)?.[1] ?? ''));
    else cells.push(value);
  }
  return cells.map((each) => each.trim()).filter(Boolean);
}

/** Bytes as base64, the way `CiFileImport.content` carries a workbook. */
export function base64Of(bytes: Uint8Array): string {
  let binary = '';
  const chunk = 0x8000;
  for (let at = 0; at < bytes.length; at += chunk) binary += String.fromCharCode(...bytes.subarray(at, at + chunk));
  return btoa(binary);
}
