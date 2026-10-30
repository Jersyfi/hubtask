// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// An entry's files in a store (UC-INT-14): the paths, the stores a link may be in, and how a link
// opens - pure, so that it is tested without a server.

import type { FileLink } from '@hubtask/sync-engine';

export const fileLinksPath = (itemId: string) => `/items/${itemId}/file-links`;
export const fileLinkPath = (linkId: string) => `/file-links/${linkId}`;

/** The stores a link may be in, in the order the form offers them; empty is "told by the address". */
export const STORES: readonly FileLink['provider'][] = ['NEXTCLOUD', 'SHAREPOINT', 'ONEDRIVE', 'WEBDAV'];

/** The address a link opens: only an https one - what a server kept is not trusted further. */
export function fileHref(address: string): string | undefined {
  try {
    const parsed = new URL(address);
    return parsed.protocol === 'https:' ? parsed.href : undefined;
  } catch {
    return undefined;
  }
}

/** The host a link lives on, said beside its name so that a reader knows where it opens. */
export function fileHost(address: string): string {
  try {
    return new URL(address).host;
  } catch {
    return '';
  }
}
