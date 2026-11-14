// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The usual kinds of work a workspace starts with (user-experience.md §4): written by the first
 * holder of `STRUCTURE` who opens Time while the workspace has none, in that reader's language, so
 * the timer starts on first use. Each stays the workspace's own to rename, archive or reorder.
 */
export const DEFAULT_KINDS: readonly { readonly key: string; readonly code: string; readonly billable: boolean }[] = [
  { key: 'development', code: 'app.time.kind_development', billable: true },
  { key: 'meeting', code: 'app.time.kind_meeting', billable: false },
  { key: 'support', code: 'app.time.kind_support', billable: true },
  { key: 'administration', code: 'app.time.kind_administration', billable: false },
];

/**
 * Whether the usual kinds are owed: the kinds were read and there are none at all - archived ones
 * count, a workspace that had kinds keeps its choice - and the reader may write them.
 */
export function owesDefaultKinds(kinds: readonly unknown[] | undefined, mayShape: boolean): boolean {
  return kinds !== undefined && kinds.length === 0 && mayShape;
}

/**
 * Whether Time has never been used here: the kinds were read and there are none at all, and the
 * shown week holds nothing. Its start page stands then (scene emptytime), and nothing is written
 * before the reader takes a way from it.
 */
export function isTimeUnused(kinds: readonly unknown[] | undefined, records: number | undefined): boolean {
  return kinds !== undefined && kinds.length === 0 && records === 0;
}

/** Writes the usual kinds, in order, each in the reader's words. */
export async function writeDefaultKinds(
  define: (key: string, fields: { name: string; billable: boolean; position: number }) => Promise<unknown>,
  word: (code: string) => string,
): Promise<void> {
  for (const [at, kind] of DEFAULT_KINDS.entries()) {
    await define(kind.key, { name: word(kind.code), billable: kind.billable, position: at + 1 });
  }
}
