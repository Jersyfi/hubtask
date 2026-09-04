// SPDX-License-Identifier: BUSL-1.1
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
  permissionVerdict,
  rootTypes as rootTypesOf,
} from './capability.ts';
import { manifest } from './capabilities.svelte.ts';

/** Whether the type carries the capability, according to this installation. */
export const supports = (type: string, capability: string): Verdict =>
  capabilityVerdict(manifest.value, type, capability);

/** Whether a child of this type may be created there, type and depth both. */
export const acceptsChild = (parentType: string, childType: string, parentDepth = 0): Verdict =>
  childVerdict(manifest.value, parentType, childType, parentDepth);

/** What may be created directly in a collection: the types nothing else claims as a child. */
export const rootTypes = (): readonly string[] => rootTypesOf(manifest.value);

/** The child types the manifest permits under this one. */
export const childTypes = (type: string): readonly string[] =>
  allowedChildTypes(manifest.value, type);

/** Whether the role carries the permission unqualified. */
export const holds = (role: string | undefined, permission: string): Verdict =>
  permissionVerdict(manifest.value, role, permission);

/** Whether this entry may be changed, `ASSIGNED` taken into account. */
export const mayChange = (role: string | undefined, isAssignedToActor: boolean): Verdict =>
  changeVerdict(manifest.value, role, isAssignedToActor);
