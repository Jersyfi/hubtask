// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The writes of an entry's file links (UC-INT-14).

import type { FileLink } from '@hubtask/sync-engine';

import { fileLinkPath, fileLinksPath } from './files.ts';
import { engine } from './engine.ts';

export const files = {
  link(itemId: string, address: string, provider: FileLink['provider'] | '', name: string): Promise<FileLink> {
    const body: Record<string, string> = { address };
    if (provider) body.provider = provider;
    if (name) body.name = name;
    return engine.mutate<FileLink>('POST', fileLinksPath(itemId), body, { invalidates: [fileLinksPath(itemId)] });
  },
  unlink(link: FileLink): Promise<void> {
    return engine.mutate<void>('DELETE', fileLinkPath(link.id), undefined, { invalidates: [fileLinksPath(link.item_id)] });
  },
};
