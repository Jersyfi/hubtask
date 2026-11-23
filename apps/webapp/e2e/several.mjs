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
 * Where the one open list stands against its field: the list never covers the field and never
 * leaves the viewport (`anchor.ts`, the shrunk fallback). Then End, and the option it focuses must
 * lie inside the list's scroll box and the viewport, clear of its stuck foot, while nothing behind
 * it scrolled. Answers the four for the walk to assert.
 */
export async function openBeside(page) {
  const read = () => page.evaluate(() => {
    const list = document.querySelector('[role="listbox"]');
    const surface = list?.closest('.surface');
    const field = document.querySelector('button[aria-haspopup="listbox"][aria-expanded="true"]');
    if (!surface || !field) return undefined;
    const s = surface.getBoundingClientRect();
    const f = field.getBoundingClientRect();
    const foot = surface.querySelector('.foot')?.getBoundingClientRect();
    const scrolled = [...document.querySelectorAll('*')].filter((node) => node !== surface && !surface.contains(node) && node.scrollTop > 0).length + window.scrollY;
    const covers = !(s.right <= f.left || s.left >= f.right || s.bottom <= f.top || s.top >= f.bottom);
    return { covers, inside: s.top >= 0 && s.left >= 0 && s.bottom <= innerHeight && s.right <= innerWidth, surface: [s.top, s.bottom], foot: foot ? foot.top : s.bottom, scrolled };
  });
  // The list opens with focus on its first chosen option, or its first: End is pressed from there.
  await page.waitForFunction(() => document.activeElement?.getAttribute('role') === 'option', null, { timeout: 5_000 });
  const before = await read();
  await page.keyboard.press('End');
  // The list scrolls the option clear of its stuck foot once it has focus: the walk waits, up to a
  // limit, for the last option focused and wholly seen inside the list, the viewport and above
  // the foot - what the reader would then see.
  const seen = await page.waitForFunction(() => {
    const node = document.activeElement;
    const surface = node?.closest('.surface');
    if (!surface || node.getAttribute('role') !== 'option') return false;
    const options = surface.querySelectorAll('[role="option"]');
    const box = node.getBoundingClientRect();
    const frame = surface.getBoundingClientRect();
    const foot = surface.querySelector('.foot')?.getBoundingClientRect().top ?? frame.bottom;
    return node === options[options.length - 1] && box.top >= Math.max(0, frame.top) && box.bottom <= Math.min(foot, innerHeight);
  }, null, { timeout: 2_000 }).then(() => true, () => false);
  const later = await read();
  return {
    coversField: before?.covers,
    insideViewport: before?.inside,
    endFocusesVisibleOption: seen,
    behindScrolled: (later?.scrolled ?? 0) - (before?.scrolled ?? 0),
  };
}

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
