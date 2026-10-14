// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The manifest's answers, bound to the one manifest this application holds.
 *
 * Three lines of binding around `capability.ts`, and that is the whole of it: the reasoning lives
 * in the pure module where it can be tested without mounting anything, and this is what hands it
 * the real document. The same split `resource.svelte.ts` makes around the engine.
 */

import type { Verdict } from './capability.ts';
import {
  allowedChildTypes,
  capabilityVerdict,
  changeVerdict,
  childVerdict,
  declaresType as declaresTypeIn,
  itemPermitted,
  holdsOnWorkspace,
  permissionVerdict,
  reachesAdministration,
  rootTypes as rootTypesOf,
  typesWith as typesWithOf,
  workspaceVerdict,
} from './capability.ts';
import { manifest } from './capabilities.svelte.ts';

/** Whether the type carries the capability, according to this installation. */
export const supports = (type: string, capability: string): Verdict =>
  capabilityVerdict(manifest.value, type, capability);

/** Whether this installation declares the type at all. `undefined` while the manifest is unread. */
export const declaresType = (type: string): boolean | undefined =>
  declaresTypeIn(manifest.value, type);

/** Whether a child of this type may be created there, type and depth both. */
export const acceptsChild = (parentType: string, childType: string, parentDepth = 0): Verdict =>
  childVerdict(manifest.value, parentType, childType, parentDepth);

/** What may be created directly in a collection: the types nothing else claims as a child. */
export const rootTypes = (): readonly string[] => rootTypesOf(manifest.value);

/** The types this installation gives a capability. What an `applies_to` chooser offers. */
export const typesWith = (capability: string): readonly string[] =>
  typesWithOf(manifest.value, capability);

/** The child types the manifest permits under this one. */
export const childTypes = (type: string): readonly string[] =>
  allowedChildTypes(manifest.value, type);

/** Whether the role carries the permission unqualified. */
export const holds = (role: string | undefined, permission: string): Verdict =>
  permissionVerdict(manifest.value, role, permission);

/** Whether the reader holds the permission on the workspace itself; `undefined` while unknown. */
export const holdsHere = (permission: string): boolean | undefined =>
  holdsOnWorkspace(manifest.value, permission);

/** Whether the reader may do what asks this permission on the workspace, as a gate's verdict. */
export const mayHere = (permission: string): Verdict => workspaceVerdict(manifest.value, permission);

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
