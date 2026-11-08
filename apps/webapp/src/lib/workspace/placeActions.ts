// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A hub's or a collection's own menu (`placeMenu`) for the reader at that place, and what picking
 * an item does from anywhere but the place's own screen: its settings open, every other act is
 * left for the place's screen (`commands.intend`), which runs it as its own head would. One answer
 * for the sidebar's row ⋯ and a collection's head on every one of its views.
 */

import type { MenuItem } from '@hubtask/design-system/components';

import { actor } from '../data/account.svelte.ts';
import { holds, roleOrder } from '../data/capability.svelte.ts';
import { archivalOf } from '../data/containers.ts';
import { containers } from '../data/containers.svelte.ts';
import { people } from '../data/people.svelte.ts';
import { workspace } from '../data/workspace.svelte.ts';
import { t } from '../i18n/i18n.svelte.ts';
import { commands } from '../palette/commands.svelte.ts';
import { placeMenu } from './placeMenu.ts';
import { ownsHub, privacyAction } from './privacy.ts';

export interface PlaceActions {
  readonly items: readonly MenuItem[];
  readonly onselect: (act: string) => void;
}

export interface PlaceActionsOptions {
  /** Whether *Change position* is offered (`placeMenu`). */
  readonly offersPosition?: boolean;
  readonly onnavigate: (path: string) => void;
  /** Where a collection is moved without leaving the screen; else its own screen moves it. */
  readonly onmove?: (collectionId: string) => void;
}

/** The place's items for the reader, or nothing where the reader may do nothing to it. */
export function placeActions(id: string, options: PlaceActionsOptions): PlaceActions | undefined {
  const container = containers.find(id);
  if (!container) return undefined;
  const at = container.type === 'HUB' ? container.id : container.parent_id;
  if (!at) return undefined;
  const path = container.type === 'HUB' ? { hubId: at } : { hubId: at, collectionId: container.id };
  const role = people.roleOf(path, actor.account?.id, roleOrder()) ?? undefined;
  const workspaceRole = people.along({}).find((each) => each.account_id === actor.account?.id && each.scope_type === 'TENANT')?.role;
  const items = placeMenu(
    {
      type: container.type === 'HUB' ? 'HUB' : 'COLLECTION',
      mayShape: holds(role, 'STRUCTURE').status === 'permitted',
      mayDelete: holds(role, 'DELETE_CONTAINER').status === 'permitted',
      archival: archivalOf(container),
      offersPosition: options.offersPosition,
      privacy:
        container.type === 'HUB'
          ? privacyAction({
              isPrivate: container.private === true,
              ownsHub: ownsHub(people.along(path), actor.account?.id, container.id),
              mayCreateHubs: holds(workspaceRole as string | undefined, 'STRUCTURE').status === 'permitted',
              isOffered: workspace.workspace?.private_hubs_offered === true,
            })
          : undefined,
    },
    (code) => t(code),
  );
  if (items.length === 0) return undefined;
  return {
    items,
    onselect: (act) => {
      const place = container.type === 'HUB' ? `/hubs/${container.id}` : `/collections/${container.id}`;
      if (act === 'settings') return options.onnavigate(`${place}/settings`);
      if (act === 'move' && container.type === 'COLLECTION' && options.onmove) return options.onmove(container.id);
      if (act) commands.intend({ containerId: container.id, act });
      options.onnavigate(place);
    },
  };
}
