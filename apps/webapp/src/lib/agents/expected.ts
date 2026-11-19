// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A test case's expected answer as a person reads it: its keys and values as fields, never JSON.

export interface ExpectedField {
  readonly name: string;
  readonly value: string;
}

function valueOf(value: unknown): string {
  if (value === null || value === undefined || value === '') return '–';
  if (Array.isArray(value)) return value.length === 0 ? '–' : value.map(valueOf).join(', ');
  if (typeof value === 'object') return Object.entries(value).map(([key, each]) => `${key}: ${valueOf(each)}`).join(', ');
  return String(value);
}

/** The expected answer's fields in their own order; an answer that is no object is one unnamed field. */
export function expectedFields(expected: unknown): ExpectedField[] {
  if (expected === undefined) return [];
  if (expected === null || typeof expected !== 'object' || Array.isArray(expected)) return [{ name: '', value: valueOf(expected) }];
  return Object.entries(expected).map(([name, value]) => ({ name, value: valueOf(value) }));
}
