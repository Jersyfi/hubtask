// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the export dialog offers of the installation's targets: an enabled one's name, chosen by its
 * identifier - never where it points, its bucket or anything it authenticates with. The export
 * refuses a disabled target, so one is not offered (P-05).
 */

import type { Target } from '../data/backup.svelte.ts';

/** The choice that means "a target of the workspace itself", whose identifier the operator types. */
export const OWN_TARGET = 'own';

export interface TargetChoice {
  readonly value: string;
  readonly label: string;
}

export function exportTargetChoices(targets: readonly Target[]): TargetChoice[] {
  return targets
    .filter((target) => target.enabled !== false)
    .map((target) => ({ value: target.id, label: target.name }));
}
