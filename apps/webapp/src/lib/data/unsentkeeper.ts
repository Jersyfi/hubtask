// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The keeper of a live document's unsent words in the signed-in account's own store (unsentdocs.ts);
// none where the browser refuses a store - the words then live as long as the tab.

import type { UpdateKeeper } from '@hubtask/editor';

import { platform } from '../platform/index.ts';
import { actor } from './account.svelte.ts';
import { keeperIn } from './unsentdocs.ts';

export function liveKeeper(kind: string, id: string): UpdateKeeper | undefined {
  const accountId = actor.account?.id ?? platform.lastAccount();
  const storage = accountId ? platform.storageFor(accountId) : undefined;
  return storage ? keeperIn(storage, kind, id) : undefined;
}
