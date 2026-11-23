// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The dialog *Create collection* as data (user-experience.md §5.2): the kind chosen and the one
 * request a press sends. Pure, so what each kind sends is tested without a browser.
 */

import type { LabelToken } from '@hubtask/design-system';

import { presetChoice, type BuiltInKey } from '../data/presetChoice.ts';
import { lookBody, type PlaceSymbol } from './look.ts';

/** The kind chosen when the dialog opens: no preset, so the collection follows its hub, now and later. */
export const LIKE_HUB = 'hub';
/** The *Project* preset, opened on its sprint view, where the first sprint starts. */
export const SCRUM = 'scrum';
/** The *Tasks* preset that then offers the hub's portal types by name. */
export const DESK = 'desk';

export interface CollectionDraft {
  readonly parentId: string | undefined;
  readonly name: string;
  readonly description: string;
  readonly symbol: PlaceSymbol | undefined;
  readonly colour: LabelToken | undefined;
  /** `LIKE_HUB`, `SCRUM`, `DESK`, or a preset's value (Hubtask's key or a saved preset's id). */
  readonly kind: string;
}

export interface CollectionRequest {
  readonly body: {
    readonly type: 'COLLECTION';
    readonly parent_id: string | undefined;
    readonly name: string;
    readonly description: string | null;
    readonly icon: string | null;
    readonly color_token: string | null;
    readonly preset?: BuiltInKey;
    readonly preset_id?: string;
  };
  /** A desk is shaped after it is made: the hub's portal types offered by name. */
  readonly isDesk: boolean;
  /** Where the new collection opens, beyond its own page; Scrum opens on its sprint view. */
  readonly landing: (id: string) => string | undefined;
}

/** The request *Create* sends for a draft. *Like its hub* names no preset. */
export function collectionRequest(draft: CollectionDraft): CollectionRequest {
  const description = draft.description.trim();
  const preset = draft.kind === DESK ? { preset: 'TASKS' as const }
    : draft.kind === SCRUM ? { preset: 'PROJECT' as const }
      : draft.kind === LIKE_HUB ? {}
        : presetChoice(draft.kind);
  return {
    body: {
      type: 'COLLECTION',
      parent_id: draft.parentId,
      name: draft.name.trim(),
      description: description === '' ? null : description,
      ...lookBody(draft.symbol, draft.colour),
      ...preset,
    },
    isDesk: draft.kind === DESK,
    landing: (id) => (draft.kind === SCRUM ? `/collections/${id}/sprint` : undefined),
  };
}
