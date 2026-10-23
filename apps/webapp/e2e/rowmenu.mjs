// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A table row whose actions sit in one menu (design-system.md §11.5): how a walk opens it, chooses
// an item and asks which items it offers. The trigger is named "Actions for <row>".

/** The row's menu trigger. */
export function rowMenu(row) {
  return row.getByRole('button', { name: /^Actions for / });
}

/** Opens the row's menu and chooses an item by its exact name. */
export async function chooseInRow(row, name) {
  await rowMenu(row).click();
  await row.page().getByRole('menuitem', { name, exact: true }).click();
}

/** How many items of that name the row's menu offers - 0 or 1 - closing it again. */
export async function rowOffers(row, name) {
  const trigger = rowMenu(row);
  if ((await trigger.count()) === 0) return 0;
  await trigger.click();
  const page = row.page();
  await page.getByRole('menu').waitFor();
  const count = await page.getByRole('menuitem', { name, exact: true }).count();
  await page.keyboard.press('Escape');
  await page.getByRole('menu').waitFor({ state: 'hidden' });
  return count;
}

/** How many rows on the page offer an item of that name. */
export async function pageOffers(page, name) {
  const rows = page.getByRole('row');
  let total = 0;
  for (let index = 0; index < (await rows.count()); index += 1) total += await rowOffers(rows.nth(index), name);
  return total;
}

/** With the row's menu open, walks to an item by the arrow keys and presses Enter on it. */
export async function keyToItem(page, name) {
  await page.getByRole('menu').waitFor();
  for (let step = 0; step < 12; step += 1) {
    const focused = await page.evaluate(() => document.activeElement?.textContent?.trim() ?? '');
    if (focused === name) {
      await page.keyboard.press('Enter');
      return;
    }
    await page.keyboard.press('ArrowDown');
  }
  throw new Error(`no menu item ${name}`);
}
