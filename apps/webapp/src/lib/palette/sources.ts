// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the palette may list, and nothing it may not (UC-WRK-27 check 2, P-05).
 *
 * **Nothing here decides a permission of its own.** The places and settings are the navigation's
 * lists (`navigation.ts`, design-system.md §11.3) with the frame's inputs; a collection's actions
 * ask the questions the collection's own screen asks, through the functions below, which that
 * screen calls too. Every answer "not known yet" is "not listed": before the manifest or the role
 * has arrived, nothing is shown as available (P-05).
 *
 * Pure, over `lib/data/capability.ts` and a translator handed in, so it is tested without mounting
 * anything; `Palette.svelte` hands it the live answers.
 */

import type { Capabilities, Container } from '@hubtask/sync-engine';

import { itemPermitted, rootTypes } from '../data/capability.ts';
import { archivalOf } from '../data/containers.ts';
import { layoutsOffered } from '../entries/layouts.ts';
import { levelWords } from '../data/levels.ts';
import { narrowedTo } from '../data/offer.ts';
import { KEEPING, SETTINGS, account, administrationFor, instanceSection, primary, type SectionGroup } from '../navigation.ts';
import type { Area } from '../router.ts';
import type { Candidate } from './rows.ts';

/** Resolved text from a message code. The caller's `t`. */
export type Translate = (code: string, params?: Record<string, string>) => string;

/** What pressing a listed row does - the palette's own verbs, and nothing a screen does not offer. */
export type Target =
  | { readonly kind: 'route'; readonly path: string }
  | { readonly kind: 'account'; readonly action: 'help' | 'sign-out' }
  | { readonly kind: 'search'; readonly line: string }
  | { readonly kind: 'open'; readonly containerId: string; readonly layout?: string; readonly add?: boolean };

/** A candidate row with what it leads to. */
export interface Offer extends Candidate {
  readonly target: Target;
  /** ADR-0032's area, where the row is one a build may treat differently. */
  readonly area?: Area;
}

// ---------------------------------------------------------------------------------------------
// A collection: whether one may create in it
// ---------------------------------------------------------------------------------------------

/**
 * Whether an entry may be created in this collection now: the reader's role there creates, the
 * collection is not put aside (itself or under its hub), and the installation offers a type that
 * may stand at its top level. `false` while the role or the manifest is unknown.
 *
 * The three conditions of the collection's own head - its primary action is present for the role
 * and carries a reason for the other two - so the palette and the screen cannot disagree.
 */
export function mayCreateIn(
  container: Container,
  role: string | null | undefined,
  manifest: Capabilities | undefined,
): boolean {
  if (container.type !== 'COLLECTION') return false;
  if (archivalOf(container) !== 'active') return false;
  if (rootTypes(narrowedTo(manifest, container.structure?.settings)).length === 0) return false;
  return itemPermitted(manifest, role, 'create', false) === true;
}

// ---------------------------------------------------------------------------------------------
// Hubs and collections
// ---------------------------------------------------------------------------------------------

/** A container the reader sees in the tree, with its hub where it is a collection. */
export interface Placed {
  readonly container: Container;
  readonly hub?: Container;
}

/** Folded for comparing names: two names that read alike are the ones a reader cannot tell apart. */
const sameName = (a: string, b: string) => a.trim().toLocaleLowerCase() === b.trim().toLocaleLowerCase();

/**
 * The hub a row is said to be in, where it needs saying: only for a collection whose name another
 * listed container shares. Elsewhere the name is the row.
 */
export function contextOf(placed: Placed, all: readonly Placed[]): string | undefined {
  if (!placed.hub) return undefined;
  const isShared = all.some((other) => other.container.id !== placed.container.id && sameName(other.container.name, placed.container.name));
  return isShared ? placed.hub.name : undefined;
}

/** Every hub and collection as a row that opens it. Put-aside ones are not in the tree's reads. */
export function containerOffers(all: readonly Placed[]): readonly Offer[] {
  return all
    .filter((placed) => archivalOf(placed.container) !== 'archived')
    .map((placed) => ({
      kind: 'container' as const,
      id: placed.container.id,
      label: placed.container.name,
      context: contextOf(placed, all),
      target: { kind: 'open' as const, containerId: placed.container.id },
    }));
}

/**
 * What one collection the reader matched by name offers: a search inside it, each layout it is
 * shown in, and a new entry in it where `mayCreateIn` says so.
 *
 * The search names the collection by its **id**, which the search screen resolves as readily as a
 * name and which two collections of one name in two hubs cannot share.
 */
export function collectionOffers(
  placed: Placed,
  all: readonly Placed[],
  input: {
    readonly role: string | null | undefined;
    readonly manifest: Capabilities | undefined;
    readonly drawable: readonly string[];
    readonly t: Translate;
  },
): readonly Offer[] {
  const { container } = placed;
  if (container.type !== 'COLLECTION') return [];
  const { t } = input;
  const name = container.name;
  const context = contextOf(placed, all);
  const offers: Offer[] = [
    {
      kind: 'action',
      id: `search-in:${container.id}`,
      label: t('app.palette.search_in', { name }),
      context,
      target: { kind: 'search', line: `in:${container.id}` },
    },
  ];
  for (const layout of layoutsOffered(container, input.manifest, input.drawable)) {
    offers.push({
      kind: 'action',
      id: `open-as:${container.id}:${layout}`,
      label: t('app.palette.open_as', { name, layout }),
      context,
      target: { kind: 'open', containerId: container.id, layout },
    });
  }
  if (mayCreateIn(container, input.role, input.manifest)) {
    const roots = rootTypes(narrowedTo(input.manifest, container.structure?.settings));
    offers.push({
      kind: 'action',
      id: `new-in:${container.id}`,
      label: levelWords(container.structure?.settings, roots.length === 1 ? (roots[0] ?? '') : '', t).newIn(name),
      context,
      target: { kind: 'open', containerId: container.id, add: true },
    });
  }
  return offers;
}

// ---------------------------------------------------------------------------------------------
// Places and settings
// ---------------------------------------------------------------------------------------------

/** What the frame knows about the reader that decides which places and settings they reach. */
export interface Reach {
  /** `READ` on the workspace: the jumble is theirs. `undefined` while unknown. */
  readonly hasJumble: boolean | undefined;
  readonly isAdministrationReachable: boolean;
  readonly isInstanceReachable: boolean;
  readonly isSingleWorkspace: boolean;
  readonly isPrivateHubsReadable: boolean;
  readonly hasAnyPrivateHub: boolean;
  readonly hasSeveralPeople: boolean;
}

function sectionOffers(groups: readonly SectionGroup[], area: Area, section: string, t: Translate): Offer[] {
  return groups
    .filter((group) => group.id !== 'back')
    .flatMap((group) => group.rows)
    .map((row) => ({
      kind: 'place' as const,
      id: `${area}:${row.id}`,
      label: t(row.titleCode ?? row.code),
      context: section,
      area,
      target: { kind: 'route' as const, path: row.path },
    }));
}

/**
 * The places and settings pages this reader reaches, each once: the navigation's places, the
 * keeping band, the account group, Your settings, the administration where it is theirs and the
 * installation for an operator. A page reached two ways is listed under the first.
 */
export function placeOffers(reach: Reach, t: Translate): readonly Offer[] {
  const offers: Offer[] = [
    ...primary({ hasJumble: reach.hasJumble === true }).flatMap((destination): Offer[] =>
      destination.target.kind === 'route'
        ? [{ kind: 'place', id: destination.id, label: t(destination.code), area: destination.area, target: { kind: 'route', path: destination.target.path } }]
        : [],
    ),
    ...KEEPING.map((row): Offer => ({ kind: 'place', id: row.id, label: t(row.code), target: { kind: 'route', path: row.path } })),
    ...account({
      isAdministrationReachable: reach.isAdministrationReachable,
      isInstanceReachable: reach.isInstanceReachable,
      isSingleWorkspace: reach.isSingleWorkspace,
    }).map(
      (destination): Offer => ({
        kind: 'place',
        id: destination.id,
        label: t(destination.code),
        area: destination.area,
        // The one place-row with a key of its own: `?` opens help.
        shortcut: destination.id === 'help' ? 'help' : undefined,
        target:
          destination.target.kind === 'action'
            ? { kind: 'account', action: destination.target.action }
            : { kind: 'route', path: destination.target.kind === 'route' ? destination.target.path : '/' },
      }),
    ),
    ...sectionOffers(SETTINGS, 'profile', t('app.nav.profile'), t),
    ...(reach.isAdministrationReachable
      ? sectionOffers(administrationFor(reach), 'administration', t('app.admin.nav'), t)
      : []),
    ...(reach.isInstanceReachable
      ? sectionOffers(instanceSection({ isSingleWorkspace: reach.isSingleWorkspace }), 'instance', t('app.instance.nav'), t)
      : []),
  ];
  const seen = new Set<string>();
  return offers.filter((offer) => {
    const where = offer.target.kind === 'route' ? offer.target.path : `${offer.target.kind}:${offer.id}`;
    if (seen.has(where)) return false;
    seen.add(where);
    return true;
  });
}
