// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The QR encoder, held to the standard's own vectors rather than to its own output (ADR-0053).
//
// A wrong code is a silent wrong code - it scans, and yields the wrong six digits - so nothing
// here was produced by the encoder and then pasted back in. Every expected value below is either
// printed in ISO/IEC 18004 (the generator polynomial of Annex A, the Reed-Solomon worked example
// of Annex I, the format information of Annex C, the version information of Annex D, the
// codeword and remainder counts of Table 1) or is a structural invariant of the symbol that the
// standard fixes (the last alignment centre sits seven modules in from the edge; the modules that
// are not function patterns number exactly the codeword bits plus the remainder). The whole
// symbol was then read back by a real decoder - Apple's Vision framework, on the pull request
// that added this file - for every version boundary and for real provisioning URIs; that check
// is not repeatable on the Linux runner and is recorded in the pull request instead.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  MAX_VERSION,
  byteCapacity,
  dataCodewordsFor,
  dataModules,
  encode,
  errorCorrection,
  formatBits,
  generator,
  penalty,
  remainderBits,
  totalCodewords,
  versionBits,
  versionFor,
} from '../src/qr.ts';

/** α^n in GF(256) with the standard's primitive polynomial, for reading a polynomial as exponents. */
function alpha(n) {
  let x = 1;
  for (let i = 0; i < n; i++) {
    x <<= 1;
    if (x & 0x100) x ^= 0x11d;
  }
  return x;
}

test('the generator polynomial for ten codewords is the one Annex A prints', () => {
  // x^10 + α^251 x^9 + α^67 x^8 + α^46 x^7 + α^61 x^6 + α^118 x^5 + α^70 x^4 + α^64 x^3 + α^94 x^2 + α^32 x + α^45
  const exponents = [0, 251, 67, 46, 61, 118, 70, 64, 94, 32, 45];
  assert.deepEqual(generator(10), exponents.map(alpha));
});

test('the Reed-Solomon codewords of the Annex I example', () => {
  // "01234567" in numeric mode, version 1-M: the sixteen data codewords the standard derives,
  // and the ten error correction codewords it prints for them.
  const data = [0x10, 0x20, 0x0c, 0x56, 0x61, 0x80, 0xec, 0x11, 0xec, 0x11, 0xec, 0x11, 0xec, 0x11, 0xec, 0x11];
  const expected = [0xa5, 0x24, 0xd4, 0xc1, 0xed, 0x36, 0xc7, 0x87, 0x2c, 0x55];
  assert.deepEqual(errorCorrection(data, 10), expected);
});

test('the format information of Annex C', () => {
  // Level M, mask pattern 101: data 00101, BCH remainder appended, then the fixed mask applied.
  assert.equal(formatBits(0b101).toString(2).padStart(15, '0'), '100000011001110');
});

test('the version information of Annex D', () => {
  assert.equal(versionBits(7).toString(2).padStart(18, '0'), '000111110010010100');
});

test('the tables agree with Table 1 of the standard', () => {
  // Total codewords per version, as the standard lists them.
  const total = [26, 44, 70, 100, 134, 172, 196, 242, 292, 346, 404, 466, 532];
  // Byte-mode capacity at level M, as the standard lists it.
  const capacity = [14, 26, 42, 62, 84, 106, 122, 152, 180, 213, 251, 287, 331];
  for (let version = 1; version <= MAX_VERSION; version++) {
    assert.equal(totalCodewords(version), total[version - 1], `version ${version} codewords`);
    assert.equal(byteCapacity(version), capacity[version - 1], `version ${version} capacity`);
    // Every module that is not a function pattern carries a bit, and there are exactly as many
    // of them as the codewords need plus the remainder: the reserved map and the block table
    // cannot disagree with each other without this failing.
    assert.equal(
      dataModules(version),
      totalCodewords(version) * 8 + remainderBits(version),
      `version ${version} data modules`,
    );
  }
});

test('the smallest version that fits is chosen, and past the tables nothing is', () => {
  assert.equal(versionFor(14), 1);
  assert.equal(versionFor(15), 2);
  assert.equal(versionFor(62), 4);
  assert.equal(versionFor(63), 5);
  assert.equal(versionFor(331), 13);
  assert.equal(versionFor(332), undefined);
  assert.throws(() => encode('x'.repeat(332)), RangeError);
});

test('the data codewords of a byte-mode payload: header, data, terminator, pad bytes', () => {
  // Version 1 holds sixteen data codewords. "Hi" is 0100 (byte mode), 00000010 (two), the two
  // bytes, a four-bit terminator, and then the pad codewords 11101100 and 00010001 alternating.
  const codewords = dataCodewordsFor(new TextEncoder().encode('Hi'), 1);
  assert.deepEqual(codewords, [
    0x40, 0x24, 0x86, 0x90, 0xec, 0x11, 0xec, 0x11, 0xec, 0x11, 0xec, 0x11, 0xec, 0x11, 0xec, 0x11,
  ]);
});

test('the symbol has the standard shape: finders, timing, the dark module, and the alignment centres', () => {
  for (const text of ['A', 'x'.repeat(63), 'y'.repeat(213), 'z'.repeat(331)]) {
    const { size, modules, version } = encode(text);
    assert.equal(size, 17 + 4 * version);
    // The centre of each finder is dark and its ring at distance two is light.
    for (const [r, c] of [[3, 3], [3, size - 4], [size - 4, 3]]) {
      assert.equal(modules[r][c], true, `finder centre at ${r},${c}`);
      assert.equal(modules[r][c + 2], false, `finder ring at ${r},${c + 2}`);
    }
    // The separators are light.
    assert.equal(modules[7][7], false);
    assert.equal(modules[7][size - 8], false);
    assert.equal(modules[size - 8][7], false);
    // Timing alternates, starting dark.
    for (let i = 8; i < size - 8; i++) {
      assert.equal(modules[6][i], i % 2 === 0, `timing row at ${i}`);
      assert.equal(modules[i][6], i % 2 === 0, `timing column at ${i}`);
    }
    assert.equal(modules[size - 8][8], true, 'the dark module');
    if (version >= 2) {
      // The last alignment pattern's centre sits seven modules in from the far edge, so its
      // 5×5 is dark, light, dark from the outside in. This is the invariant the version-10 row
      // was first written against and failed - the table said 52 where the symbol has 50.
      const centre = size - 7;
      assert.equal(modules[centre][centre], true);
      assert.equal(modules[centre - 1][centre], false);
      assert.equal(modules[centre - 2][centre], true);
      assert.equal(modules[centre][centre - 1], false);
      assert.equal(modules[centre][centre - 2], true);
    }
  }
});

test('the penalty rules score as §7.8.3 says', () => {
  // All light, five by five: rule 1 gives three per row and per column; rule 2 gives three per
  // 2×2 block, sixteen of them; rule 4 is nine steps of five percent from half (0 % is scored
  // against 5 %, the nearer multiple) at ten each.
  const light = Array.from({ length: 5 }, () => new Array(5).fill(false));
  assert.equal(penalty(light), 5 * 3 * 2 + 16 * 3 + 9 * 10);
  // A checkerboard has no run, no block, no finder-like sequence, and 52 % dark is within the
  // first step of half.
  const board = Array.from({ length: 5 }, (_, r) => Array.from({ length: 5 }, (_, c) => (r + c) % 2 === 0));
  assert.equal(penalty(board), 0);
});

test('an encoding is deterministic and its mask is one of the eight', () => {
  const first = encode('otpauth://totp/Hubtask:ada%40example.org?secret=ABCDEFGHIJKLMNOPQRSTUVWXYZ234567');
  const second = encode('otpauth://totp/Hubtask:ada%40example.org?secret=ABCDEFGHIJKLMNOPQRSTUVWXYZ234567');
  assert.deepEqual(first, second);
  assert.ok(first.mask >= 0 && first.mask < 8);
});
