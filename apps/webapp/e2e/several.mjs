// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the walks read off `MultiSelect`, the several-of-many of every screen (design-system.md
// §4): where focus is, what a control is described by, what its closed field shows.

/** The element that has focus, as `role "name"` - never the page's body. */
export function focused(page) {
  return page.evaluate(() => {
    const node = document.activeElement;
    if (!node || node === document.body) return 'BODY';
    const label = node.getAttribute('aria-labelledby')?.split(' ').map((id) => document.getElementById(id)?.textContent?.trim() ?? '').join(' ');
    return `${node.getAttribute('role') ?? node.tagName.toLowerCase()} "${(label ?? node.getAttribute('aria-label') ?? node.textContent ?? '').replace(/\s+/gu, ' ').trim()}"`;
  });
}

/** The text a control is described by, its `aria-describedby` read in order. */
export function describedBy(control) {
  return control.evaluate((node) =>
    (node.getAttribute('aria-describedby') ?? '').split(' ').filter(Boolean).map((id) => document.getElementById(id)?.textContent?.trim() ?? '').join(' '),
  );
}

/** The chips a closed field draws, in their order. */
export const chipsOf = (field) => field.locator('.chips:not(.measure) .chip').allTextContents();

/** What a closed field says: its chips, its sentence, or what it says with nothing chosen. */
export const valueOf = async (field) => (await field.locator('.value').textContent()).replace(/\s+/gu, ' ').trim();

/**
 * Presses each named option of a closed field as a person does: a press opens the list, each
 * option is pressed by its name, and Escape closes the list onto the field. Answers the names of
 * the options selected when it closed.
 */
export async function pick(page, field, names) {
  await field.click();
  // The one list open: a second open list would make this ambiguous and fail.
  const list = page.getByRole('listbox');
  await list.waitFor({ timeout: 5_000 });
  for (const name of names) await list.getByRole('option', { name, exact: true }).click();
  const chosen = await list.getByRole('option', { selected: true }).evaluateAll((options) => options.map((option) => document.getElementById(option.getAttribute('aria-labelledby'))?.textContent?.trim()));
  await page.keyboard.press('Escape');
  await list.waitFor({ state: 'detached', timeout: 5_000 });
  return chosen;
}
