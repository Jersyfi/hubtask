// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The quiet *new* mark on a product's row (user-experience.md §2.8): set when the reader switches
 * the module on, gone once its home is first visited. Kept on this device, as the reader's own
 * trace of what they just switched on: nothing about it is the account's.
 */

const KEY = 'hubtask.products.new';

/** The modules marked, read leniently: anything that is not a list of names is no mark at all. */
export function readMarks(stored: string | null | undefined): string[] {
  if (!stored) return [];
  try {
    const parsed: unknown = JSON.parse(stored);
    return Array.isArray(parsed) ? parsed.filter((each): each is string => typeof each === 'string') : [];
  } catch {
    return [];
  }
}

export const marked = (marks: readonly string[], module: string): string[] => (marks.includes(module) ? [...marks] : [...marks, module]);
export const unmarked = (marks: readonly string[], module: string): string[] => marks.filter((each) => each !== module);

export const MARKS_KEY = KEY;
