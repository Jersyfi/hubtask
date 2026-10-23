// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a collection offers: the installation's manifest narrowed to the collection's structure in
 * force (UC-WRK-23 check 4, `domain-model.md` §2 "Levels and presets").
 *
 * A collection never widens what the installation carries, so the narrowed manifest is the same
 * document with less in it - and every answer `capability.ts` gives over a manifest is then the
 * collection's answer too, with no second copy of the rules. The server resolves the structure;
 * this only applies it: the levels it offers, the fields each level carries, and the views.
 *
 * Pure, for the reason `capability.ts` is: the answers are the interesting part.
 */

import type { Capabilities, StructureSettings } from '@hubtask/sync-engine';

import { capabilityVerdict, childVerdict, placementVerdict, rootTypes, type Verdict } from './capability.ts';

/** Completion is part of every level the structure offers: it cannot be left out (`StructureSettings`). */
const ALWAYS = 'COMPLETION';

/**
 * The manifest as this structure narrows it, or the manifest itself where no structure is known.
 *
 * A level is kept when it is offered **and reachable** from an offered top level through offered
 * levels: an installation's activity sits under a work package and a task, and with neither of
 * them offered it would otherwise become a level of its own at the top. A type left out of
 * `fields` carries everything the installation gives it.
 */
export function narrowedTo(
  manifest: Capabilities | undefined,
  settings: StructureSettings | undefined,
): Capabilities | undefined {
  if (!manifest || !settings?.levels || settings.levels.length === 0) return manifest;
  const declared = manifest.item_types ?? [];
  const offered = new Set<string>(settings.levels.map((level) => level.type as string));

  const reachable = new Set<string>(rootTypes(manifest).filter((type) => offered.has(type)));
  const queue = [...reachable];
  while (queue.length > 0) {
    const type = queue.shift() as string;
    const children = (declared.find((entry) => entry.type === type)?.allowed_child_types ?? []) as readonly string[];
    for (const child of children) {
      if (!offered.has(child) || reachable.has(child)) continue;
      reachable.add(child);
      queue.push(child);
    }
  }

  const item_types = declared
    .filter((entry) => reachable.has(entry.type as string))
    .map((entry) => {
      const chosen = settings.fields?.[entry.type as string]?.capabilities;
      const capabilities = chosen
        ? (entry.capabilities ?? []).filter((capability) => capability === ALWAYS || chosen.includes(capability))
        : entry.capabilities;
      const allowed_child_types = ((entry.allowed_child_types ?? []) as readonly string[]).filter((child) => reachable.has(child));
      return { ...entry, capabilities, allowed_child_types } as typeof entry;
    });

  const views = settings.views?.offered as readonly string[] | undefined;
  const view_layouts = views
    ? ((manifest.view_layouts ?? []) as readonly string[]).filter((layout) => views.includes(layout))
    : manifest.view_layouts;

  return { ...manifest, item_types, view_layouts } as Capabilities;
}

/**
 * The installation's verdict, unless it permits and the collection does not: then the server's
 * own words for what a collection hides (`items.level_hidden`, `items.capability_hidden`) - the
 * value stays stored, and the reader learns where it can be offered again.
 */
function hiddenUnless(base: Verdict, narrowed: Verdict, isLevelOffered: boolean, params: Record<string, string>): Verdict {
  if (base.status !== 'permitted' || narrowed.status === 'permitted') return base.status === 'permitted' ? narrowed : base;
  if (!isLevelOffered) return { status: 'refused', code: 'items.level_hidden', params: { item_type: params.item_type ?? '' } };
  if (params.capability) return { status: 'refused', code: 'items.capability_hidden', params };
  return narrowed;
}

const offersType = (narrowed: Capabilities | undefined, type: string): boolean =>
  (narrowed?.item_types ?? []).some((entry) => entry.type === type);

/** Whether an entry of this type carries the capability in this collection. */
export function offerVerdict(
  manifest: Capabilities | undefined,
  settings: StructureSettings | undefined,
  type: string,
  capability: string,
): Verdict {
  const narrowed = narrowedTo(manifest, settings);
  return hiddenUnless(
    capabilityVerdict(manifest, type, capability),
    capabilityVerdict(narrowed, type, capability),
    offersType(narrowed, type),
    { item_type: type, capability },
  );
}

/** Whether a child of this type may be created under that one in this collection. */
export function offeredChild(
  manifest: Capabilities | undefined,
  settings: StructureSettings | undefined,
  parentType: string,
  childType: string,
  parentDepth = 0,
): Verdict {
  const narrowed = narrowedTo(manifest, settings);
  return hiddenUnless(
    childVerdict(manifest, parentType, childType, parentDepth),
    childVerdict(narrowed, parentType, childType, parentDepth),
    offersType(narrowed, childType),
    { item_type: childType },
  );
}

/** Whether an entry of this type may be placed there in this collection. */
export function offeredPlacement(
  manifest: Capabilities | undefined,
  settings: StructureSettings | undefined,
  type: string,
  destination: { readonly type: string; readonly depth: number } | null | undefined,
): Verdict {
  const narrowed = narrowedTo(manifest, settings);
  return hiddenUnless(
    placementVerdict(manifest, type, destination),
    placementVerdict(narrowed, type, destination),
    offersType(narrowed, type),
    { item_type: type },
  );
}
