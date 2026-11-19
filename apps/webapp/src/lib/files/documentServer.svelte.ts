// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { DocumentServer, DocumentServerCheck } from '@hubtask/sync-engine';

import { engine } from '../data/engine.ts';

/** Read by every member - the viewer asks whether an office file shows as a PDF. */
export const documentServerPath = '/media:document-server';

export const documentServers = {
  /** Stores the choice; STRUCTURE at the workspace. */
  set(choice: DocumentServer): Promise<DocumentServer> {
    return engine.mutate<DocumentServer>('PUT', documentServerPath, choice, { invalidates: [documentServerPath] });
  },
  /** Asks the server's discovery and stores nothing. */
  check(choice: DocumentServer): Promise<DocumentServerCheck> {
    return engine.mutate<DocumentServerCheck>('POST', '/media:document-server-check', choice, { invalidates: [] });
  },
};
