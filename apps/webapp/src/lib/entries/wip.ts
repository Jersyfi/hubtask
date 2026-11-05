// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Whether one more card takes a column over its limit (UC-WRK-31 check 3). A count the query left
 * out (past its time bound) is not zero: the cards the board holds are the least the column has.
 */
export function goesOverLimit(
  limit: number | null | undefined,
  counted: number | null | undefined,
  held: number,
): boolean {
  if (limit === null || limit === undefined) return false;
  return (counted ?? held) + 1 > limit;
}
