// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *People*'s one primary action with its three ways (ux-inventory §E8): how a walk opens one. The
// invitation is the button itself; the other two are in the list beside it, which joins the head's
// menu on a phone under the same name. Each way is a dialog (page-system §5.1), named by its title.

export const INVITE = 'Invite somebody';
export const WITHOUT_ADDRESS = 'Add someone without an address';
export const GRANT = 'Give somebody a role';

/**
 * The list of the ways: beside the button, or on a phone in the bar's ⋯, which carries the head's
 * menu there (the navigation concept §9).
 */
async function waysTrigger(page) {
  const beside = page.getByRole('button', { name: 'Ways to add somebody', exact: true });
  if ((await beside.count()) > 0) return beside;
  const bar = page.locator('[data-bar="menu"] button');
  return (await bar.count()) > 0 ? bar.first() : beside;
}

/** Opens the form of one way in, by the name its item and its heading carry. */
export async function addPeople(page, way = INVITE) {
  if (way === INVITE) {
    await page.getByRole('button', { name: 'Add people', exact: true }).click();
  } else {
    await (await waysTrigger(page)).click();
    await page.getByRole('menuitem', { name: way, exact: true }).click();
  }
  await page.getByRole('heading', { name: way }).waitFor();
}

/** The open way's dialog, by its title. */
export const wayDialog = (page, way = INVITE) => page.getByRole('dialog', { name: way });

/** Which ways the head offers, by name: none where nothing may be given. */
export async function waysOffered(page) {
  const trigger = await waysTrigger(page);
  if ((await trigger.count()) === 0) return [];
  await trigger.click();
  await page.getByRole('menu').waitFor();
  const names = (await page.getByRole('menuitem').allTextContents()).map((name) => name.trim());
  await page.keyboard.press('Escape');
  await page.getByRole('menu').waitFor({ state: 'hidden' });
  return names;
}
