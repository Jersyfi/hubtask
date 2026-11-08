// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A table row whose actions sit in one menu (design-system.md §11.5): how a walk opens it, chooses
// an item and asks which items it offers. The trigger is named "Actions for <row>", or, on a
// DataTable's row, "More for <row>".

/** The row's menu trigger. */
export function rowMenu(row) {
  return row.getByRole('button', { name: /^(Actions|More) for / });
}

/** The row's item of that name: in its menu, or in the sheet a DataTable opens on a phone. */
function itemOf(page, name) {
  return page.getByRole('menuitem', { name, exact: true }).or(page.locator('dialog.sheet[open]').getByRole('button', { name, exact: true }));
}

/** Opens the row's menu and chooses an item by its exact name. */
export async function chooseInRow(row, name) {
  await rowMenu(row).click();
  await itemOf(row.page(), name).click();
}

/** How many items of that name the row's menu offers - 0 or 1 - closing it again. */
export async function rowOffers(row, name) {
  const trigger = rowMenu(row);
  if ((await trigger.count()) === 0) return 0;
  await trigger.click();
  const page = row.page();
  const open = page.getByRole('menu').or(page.locator('dialog.sheet[open]'));
  await open.waitFor();
  const count = await itemOf(page, name).count();
  await page.keyboard.press('Escape');
  await open.waitFor({ state: 'hidden' });
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

/**
 * Presses a page head's act: its button where it shows, else the item of the head's ⋯ on a phone -
 * whose trigger is named after the first secondary act when the head has no menu of its own.
 */
export async function pressHead(page, name) {
  await page.getByRole('main').getByRole('heading', { level: 1 }).first().waitFor({ timeout: 15_000 });
  await page.waitForTimeout(300);
  const item = page.getByRole('menuitem', { name, exact: true });
  const button = page.getByRole('main').getByRole('button', { name, exact: true }).filter({ visible: true });
  if (await button.count() > 0) {
    await button.first().click();
    if (await button.first().getAttribute('aria-haspopup') !== null) await item.click();
    return;
  }
  await page.getByRole('main').getByRole('button', { name: /^More for / }).filter({ visible: true }).first().click();
  await item.click();
}
