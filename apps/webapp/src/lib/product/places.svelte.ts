// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The places a product's create dialog offers in its *In:* chip (user-experience.md §5.2), read
 * while the dialog is open: each hub's modules, the reader's role along each hub and, for a product
 * made at a collection, each hub's collections. The choosing is `placesToCreate`.
 */

import { untrack } from 'svelte';

import { actor } from '../data/account.svelte.ts';
import { holds, roleOrder } from '../data/capability.svelte.ts';
import { manifest } from '../data/capabilities.svelte.ts';
import { containers } from '../data/containers.svelte.ts';
import { live } from '../data/live.svelte.ts';
import { modules } from '../data/modules.svelte.ts';
import { people } from '../data/people.svelte.ts';
import { placesToCreate, type CreatePlace } from './home.ts';

/**
 * Starts the reads while `isOpen()` holds and answers the places. **Called at a component's start**:
 * its effects live as long as the component.
 */
export function createPlaces(module: string, level: 'HUB' | 'COLLECTION', isOpen: () => boolean): { readonly list: readonly CreatePlace[] } {
  let onAt = $state<Record<string, readonly string[]>>({});
  const hubs = $derived(containers.hubs.filter((hub) => !live.hasLost(hub.id)));
  const key = $derived(isOpen() ? hubs.map((hub) => hub.id).join(',') : '');
  $effect(() => {
    const ids = key === '' ? [] : key.split(',');
    return untrack(() => {
      const stops = ids.flatMap((hubId) => {
        people.open({ hubId });
        return [
          modules.here(hubId, (next) => {
            if (next.status === 'ready') onAt = { ...onAt, [hubId]: next.data.modules_on };
          }),
          ...(level === 'COLLECTION' ? [containers.openLevel(hubId)] : []),
        ];
      });
      return () => stops.forEach((stop) => stop());
    });
  });
  // A collection's own memberships count for a role there, so each collection's path is opened too.
  const collectionKey = $derived(
    level === 'COLLECTION' && key !== ''
      ? hubs.flatMap((hub) => containers.collectionsOf(hub.id).map((collection) => `${hub.id}/${collection.id}`)).join(',')
      : '',
  );
  $effect(() => {
    const paths = collectionKey === '' ? [] : collectionKey.split(',');
    untrack(() => {
      for (const path of paths) {
        const [hubId, collectionId] = path.split('/');
        people.open({ hubId, collectionId });
      }
    });
  });
  const list = $derived(
    key === ''
      ? []
      : placesToCreate({
          module,
          level,
          hubs,
          workspaceOn: manifest.value?.modules_on as readonly string[] | undefined,
          onAt,
          collectionsOf: (hubId) => containers.collectionsOf(hubId),
          mayWrite: (path) => {
            const role = people.roleOf(path, actor.account?.id, roleOrder());
            return role === undefined ? undefined : holds(role ?? undefined, 'WRITE_ITEMS').status === 'permitted';
          },
        }),
  );
  return {
    get list() {
      return list;
    },
  };
}
