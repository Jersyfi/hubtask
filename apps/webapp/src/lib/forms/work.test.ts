// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { leadingZeroBits, sha256, solve, solves } from './work.ts';

const hex = (bytes: Uint8Array) => Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('');
const text = (s: string) => new TextEncoder().encode(s);

test('the hash is SHA-256, across the block boundaries', () => {
  const vectors: [string, string][] = [
    ['', 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855'],
    ['abc', 'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad'],
    ['a'.repeat(55), '9f4390f8d30c2dd92ec9f095b65e2b9ae9b0a925a5258e241c9f1e910f734318'],
    ['a'.repeat(56), 'b35439a4ac6f0948b6d6f9e3c6af0f5f590ce20f1bde7090ef7970686ec6738a'],
    ['a'.repeat(64), 'ffe054fe7ae0cb6dc65c3af9b61d5209f439851db43d0ba5997337df154668eb'],
    ['a'.repeat(1000), '41edece42d63e8d9bf515a9ba6932e1c20cbc9f5a5d134645adb5db1b9737ea3'],
  ];
  for (const [input, digest] of vectors) assert.equal(hex(sha256(text(input))), digest, `${input.length} bytes`);
});

test('the zero bits are counted as the server counts them', () => {
  assert.equal(leadingZeroBits(new Uint8Array([0, 0, 0x10, 0xff])), 19);
  assert.equal(leadingZeroBits(new Uint8Array([0x80])), 0);
  assert.equal(leadingZeroBits(new Uint8Array(4)), 32);
});

// UC-SVC-13 check 4: the browser finds the number the server accepts - the first with 12 zero
// bits, worked out beside the test with another SHA-256.
test('the search answers the challenge', async () => {
  const challenge = '1798000000.AwMDAwMDAwMDAwMDAwMDAw.tag';
  assert.equal(await solve(challenge, 12, { chunk: 1000 }), '12472');
  assert.equal(solves(challenge, '12472', 12), true);
  assert.equal(solves(challenge, '12471', 12), false);
});
