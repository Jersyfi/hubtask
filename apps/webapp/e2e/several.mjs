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
