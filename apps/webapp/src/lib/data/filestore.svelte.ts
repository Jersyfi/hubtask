// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The writes of a person's file store connections (UC-INT-17).

import type { FileStoreConnection, FileStoreConnectionStart, FileStoreShare } from '@hubtask/sync-engine';

import { fileStorePath, fileStoresPath, type StoreKind } from './filestore.ts';
import { engine } from './engine.ts';

export const fileStores = {
  /** Starts the store's sign-in: answers the page to send the browser to, and a Nextcloud's state. */
  connect(kind: StoreKind, server?: string): Promise<FileStoreConnectionStart> {
    const body: Record<string, string> = { kind };
    if (server) body.server = server;
    return engine.mutate<FileStoreConnectionStart>('POST', `${fileStoresPath}:connect`, body, { idempotencyKey: crypto.randomUUID() });
  },
  complete(answer: { state: string; code?: string; error?: string }): Promise<FileStoreConnection> {
    return engine.mutate<FileStoreConnection>('POST', `${fileStoresPath}:complete`, answer, {
      idempotencyKey: crypto.randomUUID(), invalidates: [fileStoresPath],
    });
  },
  disconnect(conn: FileStoreConnection): Promise<void> {
    return engine.mutate<void>('DELETE', fileStorePath(conn.id), undefined, { invalidates: [fileStoresPath] });
  },
  share(connectionId: string, ref: string): Promise<FileStoreShare> {
    return engine.mutate<FileStoreShare>('POST', `${fileStorePath(connectionId)}/items:share`, { ref }, { idempotencyKey: crypto.randomUUID() });
  },
};
