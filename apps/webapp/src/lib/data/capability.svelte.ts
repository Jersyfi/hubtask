// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The manifest's answers, bound to the one manifest this application holds.
 *
 * Three lines of binding around `capability.ts`, and that is the whole of it: the reasoning lives
 * in the pure module where it can be tested without mounting anything, and this is what hands it
 * the real document. The same split `resource.svelte.ts` makes around the engine.
 */

import type { PlanFeature, Verdict } from './capability.ts';
import {
  allowedChildTypes,
  capabilityVerdict,
  changeVerdict,
  childVerdict,
  declaresType as declaresTypeIn,
  itemPermitted,
  holdsOnWorkspace,
  hubCreation,
  includedByPlan,
  permissionVerdict,
  placementVerdict,
  reachesAdministration,
  rootTypes as rootTypesOf,
  typesWith as typesWithOf,
  workspaceVerdict,
} from './capability.ts';
import { manifest } from './capabilities.svelte.ts';
import { levelRefusal, levelWords, type LevelWords } from './levels.ts';
import { narrowedTo, offerVerdict, offeredChild, offeredPlacement } from './offer.ts';
import { t } from '../i18n/i18n.svelte.ts';

import type { StructureSettings } from '@hubtask/sync-engine';

/** Whether the type carries the capability, according to this installation. */
export const supports = (type: string, capability: string): Verdict =>
  capabilityVerdict(manifest.value, type, capability);

/** Whether this installation declares the type at all. `undefined` while the manifest is unread. */
export const declaresType = (type: string): boolean | undefined =>
  declaresTypeIn(manifest.value, type);

/** Whether a child of this type may be created there, type and depth both. */
export const acceptsChild = (parentType: string, childType: string, parentDepth = 0): Verdict =>
  childVerdict(manifest.value, parentType, childType, parentDepth);

/** Whether an entry of this type may move to a destination: an entry above, or `null` for the top level. */
export const placementOf = (
  type: string,
  destination: { readonly type: string; readonly depth: number } | null | undefined,
): Verdict => placementVerdict(manifest.value, type, destination);

/** What may be created directly in a collection: the types nothing else claims as a child. */
export const rootTypes = (): readonly string[] => rootTypesOf(manifest.value);

/** The types this installation gives a capability. What an `applies_to` chooser offers. */
export const typesWith = (capability: string): readonly string[] =>
  typesWithOf(manifest.value, capability);

/** The child types the manifest permits under this one. */
export const childTypes = (type: string): readonly string[] =>
  allowedChildTypes(manifest.value, type);

/**
 * What a collection offers: the answers above, narrowed to its structure in force (`offer.ts`,
 * UC-WRK-23 check 4). Without a structure - a container read before the server answered one, or a
 * screen with no collection - the installation's answers stand.
 */
export interface Offer {
  supports(type: string, capability: string): Verdict;
  childTypes(type: string): readonly string[];
  rootTypes(): readonly string[];
  acceptsChild(parentType: string, childType: string, parentDepth?: number): Verdict;
  placementOf(type: string, destination: { readonly type: string; readonly depth: number } | null | undefined): Verdict;
  /** The layouts the installation reports and the collection offers, in the installation's order. */
  readonly layouts: readonly string[];
  /** A level's words here: its given name, or the catalogue's word for its type (`levels.ts`). */
  level(type: string): LevelWords;
  /** A refusal in words, its levels by the names this collection gave them. */
  refusal(verdict: { readonly code: string; readonly params?: Record<string, string> }): string;
}

export function offerIn(settings: StructureSettings | undefined): Offer {
  const narrowed = narrowedTo(manifest.value, settings);
  return {
    supports: (type, capability) => offerVerdict(manifest.value, settings, type, capability),
    childTypes: (type) => allowedChildTypes(narrowed, type),
    rootTypes: () => rootTypesOf(narrowed),
    acceptsChild: (parentType, childType, parentDepth = 0) => offeredChild(manifest.value, settings, parentType, childType, parentDepth),
    placementOf: (type, destination) => offeredPlacement(manifest.value, settings, type, destination),
    layouts: (narrowed?.view_layouts ?? []) as readonly string[],
    level: (type) => levelWords(settings, type, t),
    refusal: (verdict) => levelRefusal(settings, verdict, t),
  };
}

/** Whether the role carries the permission unqualified. */
export const holds = (role: string | undefined, permission: string): Verdict =>
  permissionVerdict(manifest.value, role, permission);

/** Whether the reader holds the permission on the workspace itself; `undefined` while unknown. */
export const holdsHere = (permission: string): boolean | undefined =>
  holdsOnWorkspace(manifest.value, permission);

/** Whether the reader may do what asks this permission on the workspace, as a gate's verdict. */
export const mayHere = (permission: string): Verdict => workspaceVerdict(manifest.value, permission);

/**
 * Why the reader may not do what asks this permission on the workspace, in words - the permission
 * by the name the roles screen gives it, never the contract's constant - or nothing where they may.
 * Before the manifest has answered, that it is still being read.
 */
export function workspaceRefusal(permission: string): string | undefined {
  const verdict = mayHere(permission);
  if (verdict.status === 'permitted') return undefined;
  if (verdict.status === 'undetermined') return t('app.gate.deciding');
  return t(verdict.code, { ...verdict.params, permission: t(`app.permissions.name.${permission.toLowerCase()}`) });
}

/** Which hub the reader may create - `shared`, `private` or `none`; `undefined` while unknown. */
export const hubCreationHere = (): 'shared' | 'private' | 'none' | undefined => hubCreation(manifest.value);

/** Whether the reader's plan includes the feature; `undefined` while the manifest is unread. */
export const includedHere = (feature: PlanFeature): boolean | undefined => includedByPlan(manifest.value, feature);

/**
 * The provider's account page, where the server answers one to this reader - the workspace's owners
 * and administrators - and `undefined` otherwise (multi-tenancy.md §4.2). The address is the
 * operator's; the label beside it is ours.
 */
export const accountLinkHere = (): string | undefined => manifest.value?.account_link ?? undefined;

/** Whether the reader reaches the administration; `undefined` while unknown. */
export const reachesAdministrationArea = (): boolean | undefined => reachesAdministration(manifest.value);

/** Whether this entry may be changed, `ASSIGNED` taken into account. */
export const mayChange = (role: string | undefined, isAssignedToActor: boolean): Verdict =>
  changeVerdict(manifest.value, role, isAssignedToActor);

/**
 * The installation's roles, strongest first, as the manifest reports them - the order the server's
 * effective role is taken in. Empty until the manifest is read, which leaves every role unknown.
 */
export const roleOrder = (): readonly string[] =>
  (manifest.value?.roles ?? []).map((description) => description.role as string | undefined).filter((role): role is string => role !== undefined);

/** Whether the reader may do this to one entry, from their effective role; `undefined` while unknown. */
export const mayDo = (
  role: string | null | undefined,
  kind: 'read' | 'create' | 'change' | 'comment',
  isAssignedToActor: boolean,
): boolean | undefined => itemPermitted(manifest.value, role, kind, isAssignedToActor);

/**
 * Why the reader may not change an entry, as the message code a screen renders, or nothing where
 * they may: still being read, changes only what is assigned to them, or reads and does not change.
 */
export function changeRefusalCode(role: string | null | undefined, isAssignedToActor: boolean): string | undefined {
  const permitted = mayDo(role, 'change', isAssignedToActor);
  if (permitted === true) return undefined;
  if (permitted === undefined) return 'app.gate.deciding';
  return mayDo(role, 'change', true) === true ? 'app.gate.change_assigned_only' : 'app.gate.change_none';
}
