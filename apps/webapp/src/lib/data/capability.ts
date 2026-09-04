// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the installation permits, read out of the manifest rather than compiled in.
 *
 * `domain-model.md` §2 is the rule this exists to keep: "setting a field whose capability is not
 * active for the type produces `ErrCapabilityNotSupported` — **not** silent ignoring." A client
 * that offers a bucket selector on a work package has built a control the server refuses, and one
 * that quietly hides it has told the person nothing. The third answer is `CapabilityGate`: the
 * control is there, it is off, and it carries the reason.
 *
 * Pure functions over a `Capabilities` document rather than a store, for the reason `structure.ts`
 * and `focus.ts` are: the interesting part is the answers, and a module that reached for the
 * manifest itself could only be tested by mounting the application. `capability.svelte.ts` is the
 * three-line binding that hands it the real one.
 *
 * **Nothing here has a default.** The extension example in §2 — a new type `MILESTONE` is a profile
 * entry and no code change — is only true if nothing in this file spells out the three types the
 * product happens to have today.
 */

import type { Capabilities } from '@hubtask/sync-engine';

/** The item type, as the manifest names it. A string, because the set grows without this client. */
export type ItemType = string;

/**
 * What a gated control is: permitted, refused for a stated reason, or not yet knowable.
 *
 * Three rather than two, and the third is the one a screen gets wrong. Until the manifest is read
 * nothing about the installation is known, so a control cannot honestly be shown as available —
 * and showing it anyway is how a screen offers six actions that disappear a moment later.
 */
export type Verdict =
  | { readonly status: 'permitted' }
  | { readonly status: 'refused'; readonly code: string; readonly params?: Record<string, string> }
  | { readonly status: 'undetermined' };

const PERMITTED: Verdict = { status: 'permitted' };
const UNDETERMINED: Verdict = { status: 'undetermined' };

const refused = (code: string, params?: Record<string, string>): Verdict => ({
  status: 'refused',
  code,
  params,
});

/**
 * The codes a prediction uses are the **server's own**, not a parallel set.
 *
 * `items.capability_not_supported`, `items.type_unsupported`, `items.parent_type_invalid` and
 * `items.depth_exceeded` are already in `locales/en.json` because the server sends them when a
 * request is refused. A gate that invented `capability.not_supported` beside them would mean one
 * fact had two sentences, and the reader would meet a different one depending on whether the
 * client saw the refusal coming. Only the two role qualifiers are this client's own, because they
 * are not refusals the server words — they are the reason a control was never offered.
 */

/** The profile the manifest declares for a type, or `undefined` where it declares none. */
function profileOf(manifest: Capabilities | undefined, type: ItemType) {
  return manifest?.item_types?.find((entry) => entry.type === type);
}

/**
 * Whether a type carries a capability.
 *
 * A type the manifest does not declare is **refused**, not permitted. Guessing in the permissive
 * direction is exactly what the manifest exists to prevent, and the `roles` half of it says so in
 * the contract's own words: "a client that does not know them offers buttons the server refuses."
 */
export function capabilityVerdict(
  manifest: Capabilities | undefined,
  type: ItemType,
  capability: string,
): Verdict {
  if (!manifest) return UNDETERMINED;

  const profile = profileOf(manifest, type);
  if (!profile) return refused('items.type_unsupported', { item_type: type });

  return profile.capabilities?.includes(capability)
    ? PERMITTED
    : refused('items.capability_not_supported', { item_type: type, capability });
}

/**
 * The types that may be created directly in a collection.
 *
 * Derived rather than named: a collection takes the types **nothing else claims as a child**. The
 * manifest says `allowed_child_types` per type, so the roots are what is left over — and that is
 * what makes `domain-model.md` §2's extension example true. A list of three names here would be a
 * list that is wrong on the installation with a fourth.
 */
export function rootTypes(manifest: Capabilities | undefined): readonly ItemType[] {
  const declared = manifest?.item_types ?? [];
  const claimed = new Set(
    declared.flatMap((entry) => (entry.allowed_child_types ?? []) as readonly string[]),
  );
  return declared
    .map((entry) => entry.type as string | undefined)
    .filter((type): type is string => type !== undefined && !claimed.has(type));
}

/** The types this one may hold. Empty for a type that holds nothing, and for one nobody declared. */
export function allowedChildTypes(
  manifest: Capabilities | undefined,
  type: ItemType,
): readonly ItemType[] {
  return profileOf(manifest, type)?.allowed_child_types ?? [];
}

/**
 * Whether a child of this type may be created under a parent of that one.
 *
 * The depth is checked as well as the type, because `max_depth` is the other half of I-W1 and a
 * client that offered a fourth level would be offering a refusal.
 */
export function childVerdict(
  manifest: Capabilities | undefined,
  parentType: ItemType,
  childType: ItemType,
  parentDepth = 0,
): Verdict {
  if (!manifest) return UNDETERMINED;

  const profile = profileOf(manifest, parentType);
  if (!profile) return refused('items.type_unsupported', { item_type: parentType });

  // Compared as strings, not against the generated union. `ItemType` is an enum in the contract, so
  // `@hubtask/api-client` types it closed — while `domain-model.md` §2 says the set grows with the
  // installation, and "tolerant behaviour towards unknown fields" is a binding client requirement
  // (roadmap.md phase 5). Widening here is what lets an installation with a fourth type be read by
  // a client generated before it existed: unknown means refused, never a crash.
  const permittedChildren = profile.allowed_child_types as readonly string[] | undefined;
  if (!permittedChildren?.includes(childType)) {
    return refused('items.parent_type_invalid', { item_type: childType, parent_type: parentType });
  }
  const max = profile.max_depth;
  if (max !== undefined && parentDepth + 1 > max) {
    return refused('items.depth_exceeded', { item_type: childType, maximum: String(max) });
  }
  return PERMITTED;
}

/**
 * Whether a role carries a permission unqualified.
 *
 * "Unqualified" is the contract's word and it matters: two cells of the matrix are qualifiers no
 * permission name can carry — a contributor writes only what is assigned to them, a guest may
 * comment without being able to change — and those live in `item_access` rather than here.
 */
export function permissionVerdict(
  manifest: Capabilities | undefined,
  role: string | undefined,
  permission: string,
): Verdict {
  if (!manifest || role === undefined) return UNDETERMINED;

  const description = manifest.roles?.find((entry) => entry.role === role);
  if (!description) return refused('app.gate.role_unknown', { role });

  return description.permissions?.includes(permission as never)
    ? PERMITTED
    : refused('app.gate.permission_missing', { role, permission });
}

/** What a role may do to a single entry, per kind. `NONE` where the manifest says nothing. */
export function itemAccess(
  manifest: Capabilities | undefined,
  role: string | undefined,
  kind: 'read' | 'create' | 'change' | 'comment',
): 'ALL' | 'ASSIGNED' | 'NONE' | undefined {
  if (!manifest || role === undefined) return undefined;
  const description = manifest.roles?.find((entry) => entry.role === role);
  // Every kind is answered by the contract, "including the ones that are NONE: an absent one would
  // leave a client guessing, and guessing wrong in the permissive direction is what this endpoint
  // exists to prevent". A manifest that omits one is therefore read as NONE rather than as ALL.
  return description?.item_access?.[kind] ?? 'NONE';
}

/**
 * Whether an entry may be changed, taking the qualifier into account.
 *
 * `ASSIGNED` is the cell that cannot be expressed as a permission: the role may change entries,
 * but only the ones assigned to the actor. A client that read the permission alone would offer an
 * edit control on every row and have half of them refused.
 */
export function changeVerdict(
  manifest: Capabilities | undefined,
  role: string | undefined,
  isAssignedToActor: boolean,
): Verdict {
  const access = itemAccess(manifest, role, 'change');
  if (access === undefined) return UNDETERMINED;
  if (access === 'ALL') return PERMITTED;
  if (access === 'NONE') return refused('app.gate.change_none');
  return isAssignedToActor ? PERMITTED : refused('app.gate.change_assigned_only');
}
