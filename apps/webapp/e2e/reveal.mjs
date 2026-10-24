// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A row's tools - its menu, its ＋, its labels - are out of sight until the pointer is over the
// row or the focus is in it (design-system.md §11.8): still in the reading order, but with no
// box to press. A walk reaches them the way a keyboard does, by focusing the control, which
// brings it into view, and then presses it.

/** Focuses a row's tool, which draws it, and clicks it. */
export async function reveal(control) {
  await control.focus();
  await control.click();
}
