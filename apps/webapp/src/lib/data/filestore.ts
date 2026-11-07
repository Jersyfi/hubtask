// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A person's own file store connections (UC-INT-17): the paths, the folder trail a picker walks
// and what a picked file becomes as an entry's link - pure, so that it is tested without a server.

import type { FileLink, FileStoreConnection, FileStoreItem } from '@hubtask/sync-engine';

export const fileStoresPath = '/integrations/file-stores';
export const fileStorePath = (connectionId: string) => `${fileStoresPath}/${connectionId}`;
export const fileStoreFolderPath = (connectionId: string, folder: string) =>
  folder ? `${fileStorePath(connectionId)}/items?folder=${encodeURIComponent(folder)}` : `${fileStorePath(connectionId)}/items`;

/** The page Microsoft sends the browser back to, which the server names as its redirect. */
export const FILE_STORE_RETURN = '/profile/files';

export type StoreKind = FileStoreConnection['kind'];

/** The kinds in the order the screens offer them. */
export const STORE_KINDS: readonly StoreKind[] = ['NEXTCLOUD', 'MICROSOFT'];

/** One step of the folders a picker walked: its reference and what it is called; the top first. */
export interface FolderStep {
  ref: string;
  name: string;
}

/** The trail after opening a folder of the last step. */
export function enterFolder(trail: readonly FolderStep[], item: FileStoreItem): FolderStep[] {
  return item.folder ? [...trail, { ref: item.ref, name: item.name }] : [...trail];
}

/** The trail after going back to one of its steps; the top stays. */
export function backTo(trail: readonly FolderStep[], index: number): FolderStep[] {
  return trail.slice(0, Math.max(1, index + 1));
}

/** Folders first, then files, each by name in the reader's language. */
export function sortItems(items: readonly FileStoreItem[], locale: string): FileStoreItem[] {
  return [...items].sort((a, b) => (a.folder === b.folder ? a.name.localeCompare(b.name, locale) : a.folder ? -1 : 1));
}

/** What a picked file or folder becomes as an entry's link, or nothing for one without an address. */
export function linkOf(item: FileStoreItem): { address: string; provider: FileLink['provider'] | ''; name: string } | undefined {
  if (!item.address || !item.address.startsWith('https://')) return undefined;
  return { address: item.address, provider: item.provider ?? '', name: item.name };
}

/** A connection's name on a screen: the store and the login, a Nextcloud's host beside it. */
export function connectionLabel(conn: FileStoreConnection, storeName: string): string {
  if (conn.server) {
    try {
      return `${storeName} · ${conn.login}@${new URL(conn.server).host}`;
    } catch {
      return `${storeName} · ${conn.login}`;
    }
  }
  return conn.login ? `${storeName} · ${conn.login}` : storeName;
}
