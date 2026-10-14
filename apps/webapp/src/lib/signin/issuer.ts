// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { IdentityProviderPreset } from '@hubtask/sync-engine';

/**
 * The issuer a provider form holds once another provider is chosen.
 *
 * A provider with one issuer for every account has it filled in; what somebody typed stays; an
 * issuer the form filled for the provider chosen before is taken out again, because it belongs to
 * that provider and the save would refuse it as somebody else's. Called from the choice itself,
 * never from an effect: opening a stored provider sets the kind too, and its issuer must stay.
 */
export function issuerAfterChoosing(
  presets: readonly IdentityProviderPreset[],
  chosen: string,
  current: string,
): string {
  const typed = current.trim();
  const filledBefore = presets.some((each) => each.issuer && each.issuer === typed);
  if (typed !== '' && !filledBefore) return current;
  return presets.find((each) => each.kind === chosen)?.issuer ?? '';
}

/**
 * Whether the typed issuer is one of the preset's shared endpoints - its first path segment one of
 * `shared_issuer_paths` - which the server saves only with a directory list, in every mode.
 */
export function isSharedIssuer(preset: IdentityProviderPreset | undefined, issuer: string): boolean {
  return firstSegmentIn(preset?.shared_issuer_paths ?? [], issuer);
}

/**
 * Whether the typed issuer also serves personal accounts - its first path segment one of
 * `personal_issuer_paths` - where the server refuses `ANY` in every form.
 */
export function isPersonalIssuer(preset: IdentityProviderPreset | undefined, issuer: string): boolean {
  return firstSegmentIn(preset?.personal_issuer_paths ?? [], issuer);
}

/**
 * The admission modes a form offers: the ones the preset permits, without `ANY` on an issuer that
 * serves personal accounts. `ANY` stays while it is the current value - a row stored before the
 * refusal - so the form never shows a mode the row does not have; the form says why it cannot be
 * saved so, and the person chooses another.
 */
export function offeredModes<Mode extends string>(
  modes: readonly Mode[],
  personal: boolean,
  current: string,
): Mode[] {
  return modes.filter((mode) => mode !== 'ANY' || !personal || current === 'ANY');
}

function firstSegmentIn(segments: readonly string[], issuer: string): boolean {
  if (segments.length === 0) return false;
  let path: string;
  try {
    path = new URL(issuer.trim()).pathname;
  } catch {
    return false;
  }
  const first = path.replace(/^\//, '').split('/')[0]?.toLowerCase() ?? '';
  return first !== '' && segments.some((segment) => segment.toLowerCase() === first);
}

/** Which admission list a provider form draws, and whether the server requires it. */
export interface AdmissionLists {
  /** The directory list (`allowed_directories`), in the words of the preset's directory claim. */
  readonly directories: boolean;
  /** The address domains (`allowed_email_domains`), for a preset that names no directory. */
  readonly domains: boolean;
  /** Whether the drawn list is required rather than a narrowing. */
  readonly required: boolean;
}

/**
 * The list the server will read for this mode and issuer, and only that one (ADR-0071 §2): under
 * `DOMAINS` the directories or, without a directory claim, the domains; under `INVITED_ONLY` the
 * directories as an optional narrowing; and on a shared endpoint the directories in every mode,
 * required - `ANY` included, where admission reads them on a shared endpoint only.
 */
export function admissionLists(
  preset: IdentityProviderPreset | undefined,
  provisioning: string,
  shared: boolean,
): AdmissionLists {
  const claim = Boolean(preset?.directory_claim);
  const directories = claim && (shared || provisioning === 'DOMAINS' || provisioning === 'INVITED_ONLY');
  const domains = !claim && provisioning === 'DOMAINS';
  return { directories, domains, required: shared || provisioning === 'DOMAINS' };
}
