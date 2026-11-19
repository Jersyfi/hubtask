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

/**
 * Unfolds the fields with nothing set (concept round 2, §4): they wait in the rail's folded *More
 * fields* until somebody asks, so a walk that fills one asks first, as a person does.
 */
export async function unfoldFields(page) {
  // The section is registered with the rows, once the entry's type is read; looked for before
  // that, it is not there. A page without empty fields waits the bound and goes on.
  const more = 'button[aria-controls$="-more-fields"]';
  await page.locator(`[data-detail], ${more}`).first().waitFor({ state: 'attached', timeout: 15_000 }).catch(() => undefined);
  // Only the drawn ones: on a phone the rail's sections are also in the sheet, drawn when it is open.
  const toggle = page.locator(`${more}[aria-expanded="false"]`).filter({ visible: true });
  await page.waitForTimeout(100);
  for (const each of await toggle.all()) await each.click();
}
