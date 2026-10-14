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
