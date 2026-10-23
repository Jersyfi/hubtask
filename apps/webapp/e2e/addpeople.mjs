// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *People*'s one primary action with its three ways (ux-inventory §E8): how a walk opens one. The
// invitation is the button itself; the other two are in the list beside it, which joins the head's
// menu on a phone under the same name.

export const INVITE = 'Invite somebody';
export const WITHOUT_ADDRESS = 'Add someone without an address';
export const GRANT = 'Give somebody a role';

/** Opens the form of one way in, by the name its item and its heading carry. */
export async function addPeople(page, way = INVITE) {
  if (way === INVITE) {
    await page.getByRole('button', { name: 'Add people', exact: true }).click();
  } else {
    await page.getByRole('button', { name: 'Ways to add somebody', exact: true }).click();
    await page.getByRole('menuitem', { name: way, exact: true }).click();
  }
  await page.getByRole('heading', { name: way }).waitFor();
}

/** Which ways the head offers, by name: none where nothing may be given. */
export async function waysOffered(page) {
  const trigger = page.getByRole('button', { name: 'Ways to add somebody', exact: true });
  if ((await trigger.count()) === 0) return [];
  await trigger.click();
  await page.getByRole('menu').waitFor();
  const names = (await page.getByRole('menuitem').allTextContents()).map((name) => name.trim());
  await page.keyboard.press('Escape');
  await page.getByRole('menu').waitFor({ state: 'hidden' });
  return names;
}
