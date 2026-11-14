// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the file viewer does with a file (F-2), decided from the judged type and the name.
 *
 * **Two ways onto the screen, and neither opens a file as a document** (F-5, T-11). A type the
 * server marks `inline` - a picture, audio, video - is handed to the browser's decoder by its
 * download URL. Everything else the viewer can show is fetched as bytes and drawn by Hubtask: a PDF
 * page on a canvas by pdf.js without eval, text and code as text, Markdown through the editor's
 * parser into elements (never HTML), a CSV as a table, a ZIP's listing from its central directory,
 * an SVG as a picture from its bytes (an image runs no script). An office file goes to the document
 * server where the workspace has one; otherwise, and for everything else, a card says the type and
 * the size.
 */

import type { MediaObject } from '@hubtask/sync-engine';

export type ViewKind = 'image' | 'svg' | 'video' | 'audio' | 'pdf' | 'markdown' | 'csv' | 'zip' | 'text' | 'office' | 'card';

/** The largest file drawn from its bytes; a bigger one is a card, so a tab never holds a gigabyte. */
export const DRAWN_LIMIT = 50 * 1024 * 1024;
/** The largest text shown as text; past it the first part, and the card says there is more. */
export const TEXT_LIMIT = 512 * 1024;
/** Rows of a CSV drawn as a table. */
export const CSV_ROWS = 500;
/** Lines of a ZIP's listing. */
export const ZIP_ENTRIES = 1000;

const OFFICE = new Set(['docx', 'doc', 'odt', 'rtf', 'xlsx', 'xls', 'ods', 'pptx', 'ppt', 'odp', 'odg']);
const CODE = new Set([
  'txt', 'log', 'json', 'xml', 'yaml', 'yml', 'toml', 'ini', 'cfg', 'conf', 'go', 'ts', 'js', 'mjs', 'svelte', 'py', 'rb', 'java',
  'kt', 'c', 'h', 'cpp', 'cs', 'rs', 'sh', 'sql', 'css', 'html', 'htm', 'php', 'swift', 'gradle', 'properties', 'env', 'diff', 'patch',
]);

/** The file name's ending, lower-cased, or empty. */
export function endingOf(fileName: string | null | undefined): string {
  return /\.([a-z0-9]{1,10})$/i.exec(fileName ?? '')?.[1]?.toLowerCase() ?? '';
}

/** How the viewer shows this object. A file that is not READY is never drawn. */
export function viewOf(object: Pick<MediaObject, 'content_type' | 'file_name' | 'size' | 'status'> & { inline?: boolean }): ViewKind {
  if (object.status !== 'READY') return 'card';
  const type = (object.content_type ?? '').toLowerCase().split(';')[0]?.trim() ?? '';
  const ending = endingOf(object.file_name);
  if (object.inline) {
    if (type.startsWith('image/')) return 'image';
    if (type.startsWith('video/')) return 'video';
    if (type.startsWith('audio/')) return 'audio';
  }
  if (OFFICE.has(ending) || type.includes('officedocument') || type.includes('opendocument') || type === 'application/msword') return 'office';
  if (object.size > DRAWN_LIMIT) return 'card';
  if (type === 'image/svg+xml' || ending === 'svg') return 'svg';
  if (type === 'application/pdf' || ending === 'pdf') return 'pdf';
  if (type === 'text/markdown' || ending === 'md' || ending === 'markdown') return 'markdown';
  if (type === 'text/csv' || ending === 'csv' || ending === 'tsv') return 'csv';
  if (type === 'application/zip' || ending === 'zip') return 'zip';
  if (type.startsWith('text/') || type === 'application/json' || CODE.has(ending)) return 'text';
  return 'card';
}

/** Whether a kind is drawn from the fetched bytes rather than handed over by URL. */
export const isDrawnFromBytes = (kind: ViewKind): boolean => ['svg', 'pdf', 'markdown', 'csv', 'zip', 'text'].includes(kind);

/** The short type mark a file wears in the strip and on a card: its ending, or the kind. */
export function markOf(object: Pick<MediaObject, 'content_type' | 'file_name'>): string {
  const ending = endingOf(object.file_name);
  if (ending) return ending.toUpperCase();
  const sub = (object.content_type ?? '').split('/')[1]?.split(/[;+.-]/)[0] ?? '';
  return sub ? sub.toUpperCase().slice(0, 4) : '';
}

/** The mark's label colour: the scene's red PDF, blue document, green table, amber archive. */
export function markColourOf(object: Pick<MediaObject, 'content_type' | 'file_name'>): string {
  const ending = endingOf(object.file_name);
  const type = (object.content_type ?? '').toLowerCase();
  if (type.startsWith('image/')) return 'violet';
  if (type.includes('pdf') || ending === 'pdf') return 'red';
  if (type.includes('sheet') || type.includes('csv') || ['xlsx', 'xls', 'ods', 'csv'].includes(ending)) return 'green';
  if (type.includes('zip') && !OFFICE.has(ending)) return 'amber';
  if (['pptx', 'ppt', 'odp'].includes(ending) || type.includes('presentation')) return 'amber';
  return 'blue';
}

/**
 * A CSV (or TSV) as rows of cells, RFC 4180 quoting, at most `limit` rows. The separator is the
 * one the first line uses most: a German spreadsheet writes semicolons.
 */
export function parseCsv(text: string, limit = CSV_ROWS): { rows: string[][]; isCut: boolean } {
  const firstLine = text.slice(0, text.indexOf('\n') === -1 ? text.length : text.indexOf('\n'));
  const separator = [',', ';', '\t'].map((each) => [each, firstLine.split(each).length] as const).sort((a, b) => b[1] - a[1])[0]?.[0] ?? ',';
  const rows: string[][] = [];
  let row: string[] = [];
  let cell = '';
  let quoted = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (quoted) {
      if (c === '"' && text[i + 1] === '"') { cell += '"'; i++; } else if (c === '"') quoted = false; else cell += c;
      continue;
    }
    if (c === '"' && cell === '') quoted = true;
    else if (c === separator) { row.push(cell); cell = ''; }
    else if (c === '\n' || c === '\r') {
      if (c === '\r' && text[i + 1] === '\n') i++;
      row.push(cell); cell = '';
      rows.push(row); row = [];
      if (rows.length > limit) return { rows: rows.slice(0, limit), isCut: true };
    } else cell += c;
  }
  if (cell !== '' || row.length > 0) { row.push(cell); rows.push(row); }
  return { rows: rows.slice(0, limit), isCut: rows.length > limit };
}

export interface ZipEntry { readonly name: string; readonly size: number; readonly isFolder: boolean }

/**
 * A ZIP's listing from its central directory: names and uncompressed sizes, nothing unpacked.
 * Undefined for bytes that are no ZIP. ZIP64 sizes are read as the 32-bit field says (a listing,
 * not an extraction).
 */
export function zipListing(bytes: Uint8Array, limit = ZIP_ENTRIES): { entries: ZipEntry[]; total: number } | undefined {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  // The end-of-central-directory record sits in the last 22 + 65535 bytes.
  let end = -1;
  for (let i = bytes.length - 22; i >= Math.max(0, bytes.length - 22 - 0xffff); i--) {
    if (view.getUint32(i, true) === 0x06054b50) { end = i; break; }
  }
  if (end < 0) return undefined;
  const total = view.getUint16(end + 10, true);
  let at = view.getUint32(end + 16, true);
  const decoder = new TextDecoder();
  const entries: ZipEntry[] = [];
  for (let n = 0; n < total && entries.length < limit; n++) {
    if (at + 46 > bytes.length || view.getUint32(at, true) !== 0x02014b50) break;
    const size = view.getUint32(at + 24, true);
    const nameLength = view.getUint16(at + 28, true);
    const extra = view.getUint16(at + 30, true);
    const comment = view.getUint16(at + 32, true);
    const name = decoder.decode(bytes.subarray(at + 46, at + 46 + nameLength));
    entries.push({ name, size, isFolder: name.endsWith('/') });
    at += 46 + nameLength + extra + comment;
  }
  return { entries, total };
}

/** The neighbour in a list of n that ← and → move to, wrapping at either end. */
export function neighbour(index: number, count: number, step: -1 | 1): number {
  if (count <= 0) return 0;
  return (index + step + count) % count;
}
