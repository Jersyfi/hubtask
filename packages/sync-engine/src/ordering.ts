// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Fractional order keys, the server's scheme on this side of the wire
// (`core/domain/service/Ordering.go`, offline-sync.md §4.2).
//
// A reorder made offline has to name the rank it wants as a key rather than as "before that
// entry", because by the time it is pushed that entry may have moved; the key is a fractional
// index, so two devices inserting into one list keep both insertions. The scheme is David
// Greenspan's ("Implementing Fractional Indexing", 2021): an integer part whose first character
// encodes how many digits follow it, then an optional fraction, in base 62 digits in ASCII order,
// so that comparing two keys as strings compares them as numbers. This is the server's code,
// branch for branch, with the server's own tests ported: a key this mints is a key the server
// would have minted between the same neighbours.

const DIGITS = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz';
const ZERO = DIGITS[0] as string;
const LAST = DIGITS[DIGITS.length - 1] as string;
/** The first key of an empty list: integer zero, no fraction. */
export const ORDER_KEY_ZERO = 'a0';
/** The lower end of the integer space; permitted as a prefix, never as a key of its own. */
const SMALLEST_INTEGER = `A${ZERO.repeat(26)}`;

export class OrderKeyError extends Error {
  readonly detailCode: string;

  constructor(detailCode: string, message: string) {
    super(message);
    this.name = 'OrderKeyError';
    this.detailCode = detailCode;
  }
}

const digitIndex = (digit: string): number => DIGITS.indexOf(digit);

/** The length the head character declares: `a`..`z` head the non-negative integers, `A`..`Z` the negative. */
function integerLength(head: string): number {
  if (head >= 'a' && head <= 'z') return head.charCodeAt(0) - 'a'.charCodeAt(0) + 2;
  if (head >= 'A' && head <= 'Z') return 'Z'.charCodeAt(0) - head.charCodeAt(0) + 2;
  throw new OrderKeyError('ordering.key_malformed', `not an order key head: ${head}`);
}

function splitKey(key: string): [integer: string, fraction: string] {
  let length: number;
  try {
    length = integerLength(key[0] as string);
  } catch {
    return [key, ''];
  }
  if (length > key.length) return [key, ''];
  return [key.slice(0, length), key.slice(length)];
}

function validateInteger(integer: string): void {
  if (integer === '' || integerLength(integer[0] as string) !== integer.length) {
    throw new OrderKeyError('ordering.key_malformed', `malformed integer part: ${integer}`);
  }
}

export function validateOrderKey(key: string): void {
  if (key === '') return;
  for (const digit of key) {
    if (digitIndex(digit) < 0) throw new OrderKeyError('ordering.key_malformed', `malformed key: ${key}`);
  }
  if (key === SMALLEST_INTEGER) throw new OrderKeyError('ordering.key_malformed', `malformed key: ${key}`);
  const [integer, fraction] = splitKey(key);
  validateInteger(integer);
  if (fraction.endsWith(ZERO)) throw new OrderKeyError('ordering.key_malformed', `malformed key: ${key}`);
}

/** The next integer, or the empty string at the top of the space. */
function incrementInteger(integer: string): string {
  validateInteger(integer);
  const head = integer[0] as string;
  const digits = integer.slice(1).split('');
  let carry = true;
  for (let i = digits.length - 1; carry && i >= 0; i -= 1) {
    const next = digitIndex(digits[i] as string) + 1;
    if (next === DIGITS.length) {
      digits[i] = ZERO;
    } else {
      digits[i] = DIGITS[next] as string;
      carry = false;
    }
  }
  if (!carry) return head + digits.join('');
  if (head === 'Z') return ORDER_KEY_ZERO;
  if (head === 'z') return '';
  const nextHead = String.fromCharCode(head.charCodeAt(0) + 1);
  if (nextHead > 'a') digits.push(ZERO);
  else digits.shift();
  return nextHead + digits.join('');
}

/** The previous integer, or the empty string at the bottom of the space. */
function decrementInteger(integer: string): string {
  validateInteger(integer);
  const head = integer[0] as string;
  const digits = integer.slice(1).split('');
  let borrow = true;
  for (let i = digits.length - 1; borrow && i >= 0; i -= 1) {
    const previous = digitIndex(digits[i] as string) - 1;
    if (previous < 0) {
      digits[i] = LAST;
    } else {
      digits[i] = DIGITS[previous] as string;
      borrow = false;
    }
  }
  if (!borrow) return head + digits.join('');
  if (head === 'a') return `Z${LAST}`;
  if (head === 'A') return '';
  const previousHead = String.fromCharCode(head.charCodeAt(0) - 1);
  if (previousHead < 'Z') digits.push(LAST);
  else digits.shift();
  return previousHead + digits.join('');
}

const cut = (s: string, n: number): string => (n >= s.length ? '' : s.slice(n));

/** The shortest fraction strictly between a and b, where an empty b means "1". */
function midpoint(a: string, b: string): string {
  if (b !== '' && a >= b) throw new OrderKeyError('ordering.bounds_invalid', `${a} is not below ${b}`);
  if (a.endsWith(ZERO) || (b !== '' && b.endsWith(ZERO))) {
    throw new OrderKeyError('ordering.key_malformed', `trailing zero: ${a} ${b}`);
  }
  if (b !== '') {
    let common = 0;
    while (common < b.length) {
      const digit = common < a.length ? (a[common] as string) : ZERO;
      if (digit !== b[common]) break;
      common += 1;
    }
    if (common > 0) return b.slice(0, common) + midpoint(cut(a, common), b.slice(common));
  }
  const lower = a !== '' ? digitIndex(a[0] as string) : 0;
  const upper = b !== '' ? digitIndex(b[0] as string) : DIGITS.length;
  if (upper - lower > 1) return DIGITS[Math.floor((lower + upper + 1) / 2)] as string;
  if (b.length > 1) return b.slice(0, 1);
  return (DIGITS[lower] as string) + midpoint(cut(a, 1), '');
}

function keyBefore(next: string): string {
  const [integer, fraction] = splitKey(next);
  if (integer === SMALLEST_INTEGER) return integer + midpoint('', fraction);
  if (integer < next) return integer;
  const decremented = decrementInteger(integer);
  if (decremented === '') throw new OrderKeyError('ordering.space_exhausted', `nothing below ${next}`);
  return decremented;
}

function keyAfter(previous: string): string {
  const [integer, fraction] = splitKey(previous);
  const incremented = incrementInteger(integer);
  if (incremented !== '') return incremented;
  return integer + midpoint(fraction, '');
}

function keyBetween(previous: string, next: string): string {
  const [previousInteger, previousFraction] = splitKey(previous);
  const [nextInteger, nextFraction] = splitKey(next);
  if (previousInteger === nextInteger) return previousInteger + midpoint(previousFraction, nextFraction);
  const incremented = incrementInteger(previousInteger);
  if (incremented !== '' && incremented < next) return incremented;
  return previousInteger + midpoint(previousFraction, '');
}

/**
 * A key that sorts strictly after `previous` and strictly before `next`. An empty `previous`
 * means "before everything", an empty `next` "after everything"; both empty is the first key of
 * an empty list.
 */
export function orderKeyBetween(previous: string, next: string): string {
  validateOrderKey(previous);
  validateOrderKey(next);
  if (previous !== '' && next !== '' && previous >= next) {
    throw new OrderKeyError('ordering.bounds_invalid', `${previous} is not below ${next}`);
  }
  if (previous === '' && next === '') return ORDER_KEY_ZERO;
  if (previous === '') return keyBefore(next);
  if (next === '') return keyAfter(previous);
  return keyBetween(previous, next);
}

export const orderKeyAfter = (previous: string): string => orderKeyBetween(previous, '');
