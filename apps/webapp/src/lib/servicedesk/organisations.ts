// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The portal organisations' arithmetic (UC-SVC-11): mail domains from what an administrator types,
 * and whether the portal offers the organisation's requests. Pure, tested beside.
 */

import type { PortalOrganisation } from '@hubtask/sync-engine';

/** Mail domains from text - one per line or separated by commas or spaces, `@` dropped, small letters, each once. */
export function domainsOf(text: string): string[] {
  const out: string[] = [];
  for (const word of text.split(/[\s,;]+/u)) {
    const domain = word.trim().replace(/^@/u, '').toLowerCase();
    if (domain !== '' && !out.includes(domain)) out.push(domain);
  }
  return out;
}

/** Whether the portal offers *My organisation's requests*: only where the organisation shares them. */
export const offersOrganisation = (organisation: PortalOrganisation | null | undefined): boolean =>
  organisation?.shares_requests === true;
