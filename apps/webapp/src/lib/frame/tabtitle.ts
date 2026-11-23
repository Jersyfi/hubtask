// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The browser tab's title (UC-INS-22 check 9): "{page} – {workspace}" where the page told the
 * frame its title, the workspace's name where it did not, and the product's name in the
 * workspace's place wherever no workspace is known - before sign-in at the installation's own
 * address, a public page, a workspace not read yet. Pure, so the rule has a test; `App.svelte` is
 * the one place that writes it: a document attribute has one owner.
 */

/** A name rather than a message: the product is called Hubtask in every language. */
export const PRODUCT_NAME = 'Hubtask';

export function tabTitleOf(page: string | undefined, workspace: string | undefined): string {
  const owner = workspace?.trim() || PRODUCT_NAME;
  const named = page?.trim();
  return named ? `${named} – ${owner}` : owner;
}
