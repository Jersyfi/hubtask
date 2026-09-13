// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A QR encoder: a string in, a module matrix out (ISO/IEC 18004, ADR-0053 option B).
 *
 * **Byte mode, error correction level M, versions 1 to 13.** That is the whole of what it does,
 * and the bound is the input it exists for: an `otpauth://` provisioning URI. ADR-0053 wrote
 * "inside version 4", which is wrong by measurement — the URI the server actually writes carries
 * the issuer twice, the account's email address and five query parameters, URL-encoded, and is
 * around 140 bytes for an ordinary address and 220 for a long issuer with a long address, where
 * version 4 at level M holds 62. Version 13 holds 331, and 13 is also where the last version
 * with no remainder bits sits, so the placement needs no second rule. Past that the encoder
 * refuses by name rather than encoding into a version it has no tables for, and the caller shows
 * no image — the secret and the link it stands beside are the enrolment; the image is an aid.
 * The tables that vary by version — the block structure and the alignment positions — are
 * thirteen rows here rather than forty.
 *
 * **Nothing here is clever, and that is the point.** A wrong code is a *silent* wrong code: it
 * scans, an authenticator produces six digits, and they are the wrong six digits. So every step
 * is the standard's, written the plain way, and `test/qr.test.js` holds each to the standard's
 * own published vectors — the Reed–Solomon example of Annex I, the format information of Annex C,
 * the version information of Annex D — and then to a real decoder's reading of whole symbols.
 *
 * **No dependency.** The arithmetic is GF(256) with the standard's primitive polynomial, a few
 * generator polynomials, and a mask chosen by four penalty rules; it fits in one file, and the
 * lockfile does not learn that a QR code exists.
 */

/** The number of modules along one side of a version-`v` symbol: 17 + 4v. */
export type Matrix = {
  readonly size: number;
  /** Row-major; `true` is a dark module. */
  readonly modules: readonly (readonly boolean[])[];
  readonly version: number;
  readonly mask: number;
};

/** The largest version this encoder has tables for. */
export const MAX_VERSION = 13;

/** An element that is known to be there: the tables are fixed and every index below is bounded. */
function at<T>(list: ArrayLike<T>, index: number): T {
  const item = list[index];
  if (item === undefined) throw new RangeError(`qr: index ${index} is out of the table`);
  return item;
}

/** A square of modules, `true` being dark. */
class Grid {
  readonly size: number;
  private readonly cells: Uint8Array;

  // No parameter property: Node's type stripping, which runs the tests, refuses that syntax.
  constructor(size: number) {
    this.size = size;
    this.cells = new Uint8Array(size * size);
  }

  get(row: number, col: number): boolean {
    return this.cells[row * this.size + col] === 1;
  }

  set(row: number, col: number, dark: boolean): void {
    this.cells[row * this.size + col] = dark ? 1 : 0;
  }

  /** As nested arrays, for the matrix a caller reads. */
  rows(): boolean[][] {
    return Array.from({ length: this.size }, (_, r) =>
      Array.from({ length: this.size }, (_, c) => this.get(r, c)),
    );
  }
}

/**
 * The block structure at level M, by version (Table 9 of the standard, level M rows).
 *
 * `ec` is the error correction codewords per block; the blocks are listed as
 * `[count, dataCodewords]` pairs, the second group being the one with one more data codeword.
 */
const BLOCKS: readonly { ec: number; groups: readonly (readonly [number, number])[] }[] = [
  { ec: 10, groups: [[1, 16]] }, // 1-M
  { ec: 16, groups: [[1, 28]] }, // 2-M
  { ec: 26, groups: [[1, 44]] }, // 3-M
  { ec: 18, groups: [[2, 32]] }, // 4-M
  { ec: 24, groups: [[2, 43]] }, // 5-M
  { ec: 16, groups: [[4, 27]] }, // 6-M
  { ec: 18, groups: [[4, 31]] }, // 7-M
  { ec: 22, groups: [[2, 38], [2, 39]] }, // 8-M
  { ec: 22, groups: [[3, 36], [2, 37]] }, // 9-M
  { ec: 26, groups: [[4, 43], [1, 44]] }, // 10-M
  { ec: 30, groups: [[1, 50], [4, 51]] }, // 11-M
  { ec: 22, groups: [[6, 36], [2, 37]] }, // 12-M
  { ec: 22, groups: [[8, 37], [1, 38]] }, // 13-M
];

/** Alignment pattern centre coordinates by version (Annex E). Version 1 has none. */
const ALIGNMENT: readonly (readonly number[])[] = [
  [],
  [6, 18],
  [6, 22],
  [6, 26],
  [6, 30],
  [6, 34],
  [6, 22, 38],
  [6, 24, 42],
  [6, 26, 46],
  [6, 28, 50],
  [6, 30, 54],
  [6, 32, 58],
  [6, 34, 62],
];

/**
 * Remainder bits after the last codeword (Table 1): none for 1 and 7–13, seven for 2–6. The
 * placement writes them as zeros without counting; this is what the test holds the tables to.
 * Version 14 is the first with three, which is one reason the tables stop at 13.
 */
export function remainderBits(version: number): number {
  return version >= 2 && version <= 6 ? 7 : 0;
}

/** Total codewords, data and error correction, a version holds (Table 1). */
export function totalCodewords(version: number): number {
  const { ec, groups } = at(BLOCKS, version - 1);
  return dataCodewords(version) + ec * groups.reduce((sum, [count]) => sum + count, 0);
}

/** How many modules of a version carry codeword bits: everything that is not a function pattern. */
export function dataModules(version: number): number {
  const size = 17 + 4 * version;
  const isReserved = reserved(size, version);
  let count = 0;
  for (let r = 0; r < size; r++) for (let c = 0; c < size; c++) if (!isReserved.get(r, c)) count++;
  return count;
}

/** Data codewords a version holds at level M. */
export function dataCodewords(version: number): number {
  return at(BLOCKS, version - 1).groups.reduce((sum, [count, data]) => sum + count * data, 0);
}

/** Bytes of payload a version holds in byte mode at level M: the data bits less the header. */
export function byteCapacity(version: number): number {
  const header = 4 + countBits(version);
  return Math.floor((dataCodewords(version) * 8 - header) / 8);
}

/** The character count indicator's width in byte mode: 8 bits up to version 9, 16 from 10. */
function countBits(version: number): number {
  return version <= 9 ? 8 : 16;
}

/** The smallest version the payload fits, or undefined past the tables. */
export function versionFor(byteLength: number): number | undefined {
  for (let version = 1; version <= MAX_VERSION; version++) {
    if (byteLength <= byteCapacity(version)) return version;
  }
  return undefined;
}

// ---------------------------------------------------------------------------------------------
// GF(256) and Reed–Solomon (Annex A)

const EXP = new Uint8Array(512);
const LOG = new Uint8Array(256);
(() => {
  let x = 1;
  for (let i = 0; i < 255; i++) {
    EXP[i] = x;
    LOG[x] = i;
    // The field's primitive polynomial is x^8 + x^4 + x^3 + x^2 + 1 (0x11D).
    x <<= 1;
    if (x & 0x100) x ^= 0x11d;
  }
  for (let i = 255; i < 512; i++) EXP[i] = at(EXP, i - 255);
})();

function multiply(a: number, b: number): number {
  if (a === 0 || b === 0) return 0;
  return at(EXP, at(LOG, a) + at(LOG, b));
}

/** The generator polynomial for `degree` error correction codewords: Π (x − α^i), i = 0..n−1. */
export function generator(degree: number): number[] {
  let poly = [1];
  for (let i = 0; i < degree; i++) {
    const next = new Array<number>(poly.length + 1).fill(0);
    for (let j = 0; j < poly.length; j++) {
      next[j] = at(next, j) ^ at(poly, j);
      next[j + 1] = at(next, j + 1) ^ multiply(at(poly, j), at(EXP, i));
    }
    poly = next;
  }
  return poly;
}

/** The error correction codewords for one block of data codewords. */
export function errorCorrection(data: readonly number[], degree: number): number[] {
  const gen = generator(degree);
  const remainder = new Array<number>(degree).fill(0);
  for (const byte of data) {
    const factor = byte ^ at(remainder, 0);
    remainder.shift();
    remainder.push(0);
    if (factor === 0) continue;
    for (let i = 0; i < degree; i++) {
      remainder[i] = at(remainder, i) ^ multiply(at(gen, i + 1), factor);
    }
  }
  return remainder;
}

// ---------------------------------------------------------------------------------------------
// Data encoding (§7.4) and codeword assembly (§7.5, §7.6)

class BitBuffer {
  readonly bits: number[] = [];

  append(value: number, width: number): void {
    for (let i = width - 1; i >= 0; i--) this.bits.push((value >>> i) & 1);
  }

  toBytes(): number[] {
    const bytes: number[] = [];
    for (let i = 0; i < this.bits.length; i += 8) {
      let byte = 0;
      for (let j = 0; j < 8; j++) byte = (byte << 1) | (this.bits[i + j] ?? 0);
      bytes.push(byte);
    }
    return bytes;
  }
}

/** The data codewords for a UTF-8 payload in byte mode: header, data, terminator, pad bytes. */
export function dataCodewordsFor(payload: Uint8Array, version: number): number[] {
  const capacity = dataCodewords(version) * 8;
  const buffer = new BitBuffer();
  buffer.append(0b0100, 4); // byte mode
  buffer.append(payload.length, countBits(version));
  for (const byte of payload) buffer.append(byte, 8);
  // The terminator is up to four zero bits, fewer if the capacity is that close.
  buffer.append(0, Math.min(4, capacity - buffer.bits.length));
  // Then to a byte boundary, then alternate pad codewords 11101100 and 00010001.
  while (buffer.bits.length % 8 !== 0) buffer.bits.push(0);
  const pads = [0xec, 0x11];
  for (let i = 0; buffer.bits.length < capacity; i++) buffer.append(at(pads, i % 2), 8);
  return buffer.toBytes();
}

/** Data and error correction codewords, split into blocks and interleaved (§7.6). */
export function interleave(data: readonly number[], version: number): number[] {
  const { ec, groups } = at(BLOCKS, version - 1);
  const dataBlocks: number[][] = [];
  const ecBlocks: number[][] = [];
  let offset = 0;
  for (const [count, length] of groups) {
    for (let i = 0; i < count; i++) {
      const block = data.slice(offset, offset + length);
      offset += length;
      dataBlocks.push(block);
      ecBlocks.push(errorCorrection(block, ec));
    }
  }
  const out: number[] = [];
  const longest = Math.max(...dataBlocks.map((block) => block.length));
  for (let i = 0; i < longest; i++) {
    for (const block of dataBlocks) if (i < block.length) out.push(at(block, i));
  }
  for (let i = 0; i < ec; i++) {
    for (const block of ecBlocks) out.push(at(block, i));
  }
  return out;
}

// ---------------------------------------------------------------------------------------------
// Function patterns (§6.3), format and version information (§7.9, §7.10)

/** The 15-bit format information for level M and a mask: BCH(15,5) plus the fixed XOR mask. */
export function formatBits(mask: number): number {
  const data = (0b00 << 3) | mask; // level M is 00
  let remainder = data << 10;
  for (let i = 14; i >= 10; i--) {
    if (remainder & (1 << i)) remainder ^= 0b10100110111 << (i - 10);
  }
  return ((data << 10) | remainder) ^ 0b101010000010010;
}

/** The 18-bit version information for versions 7 and up: BCH(18,6). */
export function versionBits(version: number): number {
  let remainder = version << 12;
  for (let i = 17; i >= 12; i--) {
    if (remainder & (1 << i)) remainder ^= 0b1111100100101 << (i - 12);
  }
  return (version << 12) | remainder;
}

/** Which modules are function patterns, and so neither carry data nor are masked. */
function reserved(size: number, version: number): Grid {
  const grid = new Grid(size);
  const mark = (row: number, col: number, height: number, width: number): void => {
    for (let r = row; r < row + height; r++) {
      for (let c = col; c < col + width; c++) {
        if (r >= 0 && r < size && c >= 0 && c < size) grid.set(r, c, true);
      }
    }
  };
  // Finders with their separators, and the format information beside each.
  mark(0, 0, 9, 9);
  mark(0, size - 8, 9, 8);
  mark(size - 8, 0, 8, 9);
  // Timing.
  mark(6, 0, 1, size);
  mark(0, 6, size, 1);
  for (const [r, c] of alignmentCentres(version)) mark(r - 2, c - 2, 5, 5);
  // Version information, twice.
  if (version >= 7) {
    mark(0, size - 11, 6, 3);
    mark(size - 11, 0, 3, 6);
  }
  return grid;
}

/** Every alignment pattern centre of a version: each pair of coordinates, less the three a finder occupies. */
function alignmentCentres(version: number): [number, number][] {
  const centres = at(ALIGNMENT, version - 1);
  if (centres.length === 0) return [];
  const last = at(centres, centres.length - 1);
  const pairs: [number, number][] = [];
  for (const r of centres) {
    for (const c of centres) {
      const onFinder = (r === 6 && c === 6) || (r === 6 && c === last) || (c === 6 && r === last);
      if (!onFinder) pairs.push([r, c]);
    }
  }
  return pairs;
}

/** Draws the function patterns into a blank grid. */
function functionPatterns(size: number, version: number): Grid {
  const grid = new Grid(size);
  const finder = (row: number, col: number): void => {
    for (let r = -1; r <= 7; r++) {
      for (let c = -1; c <= 7; c++) {
        const rr = row + r;
        const cc = col + c;
        if (rr < 0 || rr >= size || cc < 0 || cc >= size) continue;
        const ring = Math.max(Math.abs(r - 3), Math.abs(c - 3));
        grid.set(rr, cc, ring === 2 || ring === 4 ? false : ring <= 3);
      }
    }
  };
  finder(0, 0);
  finder(0, size - 7);
  finder(size - 7, 0);
  for (let i = 8; i < size - 8; i++) {
    grid.set(6, i, i % 2 === 0);
    grid.set(i, 6, i % 2 === 0);
  }
  for (const [r, c] of alignmentCentres(version)) {
    for (let dr = -2; dr <= 2; dr++) {
      for (let dc = -2; dc <= 2; dc++) {
        const ring = Math.max(Math.abs(dr), Math.abs(dc));
        grid.set(r + dr, c + dc, ring !== 1);
      }
    }
  }
  // The dark module, always.
  grid.set(size - 8, 8, true);
  if (version >= 7) {
    const bits = versionBits(version);
    for (let i = 0; i < 18; i++) {
      const bit = ((bits >>> i) & 1) === 1;
      const a = Math.floor(i / 3);
      const b = (i % 3) + size - 11;
      grid.set(a, b, bit);
      grid.set(b, a, bit);
    }
  }
  return grid;
}

/** Writes the format information into both of its places. */
function writeFormat(grid: Grid, size: number, mask: number): void {
  const bits = formatBits(mask);
  const bit = (i: number): boolean => ((bits >>> i) & 1) === 1;
  // Around the top-left finder (§7.9.1, Figure 25): the least significant bit at the top of
  // column 8, the most significant at the left of row 8, and the timing modules stepped over.
  for (let i = 0; i <= 5; i++) grid.set(i, 8, bit(i));
  grid.set(7, 8, bit(6));
  grid.set(8, 8, bit(7));
  grid.set(8, 7, bit(8));
  for (let i = 9; i <= 14; i++) grid.set(8, 14 - i, bit(i));
  // The second copy, split between the other two finders.
  for (let i = 0; i <= 7; i++) grid.set(8, size - 1 - i, bit(i));
  for (let i = 8; i <= 14; i++) grid.set(size - 15 + i, 8, bit(i));
}

// ---------------------------------------------------------------------------------------------
// Placement (§7.7), masking (§7.8) and the penalty rules

/** The eight mask conditions, by pattern reference. */
const MASKS: readonly ((r: number, c: number) => boolean)[] = [
  (r, c) => (r + c) % 2 === 0,
  (r) => r % 2 === 0,
  (_, c) => c % 3 === 0,
  (r, c) => (r + c) % 3 === 0,
  (r, c) => (Math.floor(r / 2) + Math.floor(c / 3)) % 2 === 0,
  (r, c) => ((r * c) % 2) + ((r * c) % 3) === 0,
  (r, c) => (((r * c) % 2) + ((r * c) % 3)) % 2 === 0,
  (r, c) => (((r + c) % 2) + ((r * c) % 3)) % 2 === 0,
];

/** Places the codeword bits in the two-module-wide zigzag, skipping function modules. */
function place(grid: Grid, isReserved: Grid, size: number, codewords: readonly number[], mask: number): void {
  const bits: number[] = [];
  for (const byte of codewords) for (let i = 7; i >= 0; i--) bits.push((byte >>> i) & 1);
  const masked = at(MASKS, mask);
  let next = 0;
  let upward = true;
  for (let right = size - 1; right >= 1; right -= 2) {
    if (right === 6) right = 5; // the vertical timing column is skipped whole
    for (let step = 0; step < size; step++) {
      const row = upward ? size - 1 - step : step;
      for (const col of [right, right - 1]) {
        if (isReserved.get(row, col)) continue;
        // Past the last bit come the remainder bits, which are zero.
        const bit = next < bits.length ? at(bits, next) === 1 : false;
        next++;
        grid.set(row, col, masked(row, col) ? !bit : bit);
      }
    }
    upward = !upward;
  }
}

/** The four penalty rules of §7.8.3, N1 = 3, N2 = 3, N3 = 40, N4 = 10. */
export function penalty(modules: readonly (readonly boolean[])[]): number {
  const size = modules.length;
  const cell = (r: number, c: number): boolean => at(at(modules, r), c);
  let score = 0;

  // Rule 1: runs of five or more alike in a row or column.
  const runs = (read: (i: number, j: number) => boolean): void => {
    for (let i = 0; i < size; i++) {
      let run = 1;
      for (let j = 1; j < size; j++) {
        if (read(i, j) === read(i, j - 1)) {
          run++;
          if (run === 5) score += 3;
          else if (run > 5) score += 1;
        } else {
          run = 1;
        }
      }
    }
  };
  runs((r, c) => cell(r, c));
  runs((c, r) => cell(r, c));

  // Rule 2: every 2×2 block of one colour.
  for (let r = 0; r + 1 < size; r++) {
    for (let c = 0; c + 1 < size; c++) {
      const v = cell(r, c);
      if (cell(r, c + 1) === v && cell(r + 1, c) === v && cell(r + 1, c + 1) === v) score += 3;
    }
  }

  // Rule 3: the finder-like 1:1:3:1:1 sequence with four light modules on either side.
  const pattern = [true, false, true, true, true, false, true];
  const matches = (read: (k: number) => boolean | undefined, start: number): boolean => {
    for (let k = 0; k < 7; k++) if (read(start + k) !== at(pattern, k)) return false;
    let before = true;
    let after = true;
    for (let k = 0; k < 4; k++) {
      if (read(start - 4 + k) !== false) before = false;
      if (read(start + 7 + k) !== false) after = false;
    }
    return before || after;
  };
  const inRow = (i: number) => (k: number) => (k < 0 || k >= size ? undefined : cell(i, k));
  const inColumn = (i: number) => (k: number) => (k < 0 || k >= size ? undefined : cell(k, i));
  for (let i = 0; i < size; i++) {
    for (let j = 0; j + 7 <= size; j++) {
      if (matches(inRow(i), j)) score += 40;
      if (matches(inColumn(i), j)) score += 40;
    }
  }

  // Rule 4: the proportion of dark modules, in steps of five percent away from half.
  let dark = 0;
  for (const row of modules) for (const module of row) if (module) dark++;
  const percent = (dark * 100) / (size * size);
  const previous = Math.floor(percent / 5) * 5;
  const following = previous + 5;
  score += (Math.min(Math.abs(previous - 50), Math.abs(following - 50)) / 5) * 10;

  return score;
}

/** The symbol for one mask, complete with its format information. */
function symbol(version: number, codewords: readonly number[], mask: number): boolean[][] {
  const size = 17 + 4 * version;
  const grid = functionPatterns(size, version);
  const isReserved = reserved(size, version);
  place(grid, isReserved, size, codewords, mask);
  writeFormat(grid, size, mask);
  return grid.rows();
}

/** The whole thing: version chosen, codewords assembled, the lowest-penalty mask taken. */
export function encode(text: string): Matrix {
  const payload = new TextEncoder().encode(text);
  const version = versionFor(payload.length);
  if (version === undefined) {
    throw new RangeError(`qr: ${payload.length} bytes exceed version ${MAX_VERSION} at level M`);
  }
  const codewords = interleave(dataCodewordsFor(payload, version), version);

  let best: { modules: boolean[][]; mask: number; score: number } | undefined;
  for (let mask = 0; mask < MASKS.length; mask++) {
    const modules = symbol(version, codewords, mask);
    const score = penalty(modules);
    if (best === undefined || score < best.score) best = { modules, mask, score };
  }
  if (best === undefined) throw new Error('qr: no mask was scored');
  return { size: best.modules.length, modules: best.modules, version, mask: best.mask };
}
