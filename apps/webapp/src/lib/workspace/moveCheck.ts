// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The move pre-check's findings in words (the navigation concept's §5.3, UC-WRK-39 check 7): each
 * `MoveFinding` a message code and its parameters, never the code itself; and whether an obstacle
 * is already resolved in the dialog - a name taken, by the new name typed beside it. Pure; the
 * dialog is `CollectionMoveDialog`.
 */

import type { MoveFinding } from '@hubtask/sync-engine';

export interface FindingContext {
  /** The hub the collection leaves, and the one it would go to. */
  readonly from: string;
  readonly to: string;
  /** An account's name, for those who lose access. */
  readonly nameOf: (accountId: string) => string;
  /** A hub's name by its id, for the findings that carry one. */
  readonly hubName: (hubId: string) => string;
}

type Words = (code: string, params?: Record<string, string | number>) => string;

/** The sentence a finding says. An unknown code says that something stops the move, nothing more. */
export function findingWords(finding: MoveFinding, context: FindingContext, t: Words): string {
  const p = finding.params;
  switch (finding.code) {
    case 'containers.name_taken':
      return t('app.move.check_name_taken', { hub: context.to });
    case 'entry_types.hub_bound':
      return t('app.move.check_types', { hub: context.from });
    case 'workflows.hub_bound':
      return t('app.move.check_workflow', { hub: context.from });
    case 'access.not_permitted':
      return t('app.move.check_no_structure_in', { hub: context.hubName(p['hub_id'] ?? '') || context.to });
    case 'containers.move_loses_access': {
      const names = (p['account_ids'] ?? '').split(',').filter(Boolean).map(context.nameOf).filter(Boolean);
      const count = Number(p['count'] ?? names.length);
      return names.length > 0 ? t('app.move.check_loses_access_named', { count, names: names.join(', ') }) : t('app.move.check_loses_access', { count });
    }
    case 'containers.move_takes_structure':
      return t('app.move.check_takes_structure', { hub: context.hubName(p['hub_id'] ?? '') || context.to });
    case 'containers.move_keeps_releases':
      return t('app.move.check_keeps_releases', { count: Number(p['count'] ?? 0), hub: context.from });
    case 'containers.move_keeps_sla_policies':
      return t('app.move.check_keeps_sla', { count: Number(p['count'] ?? 0), hub: context.from });
    default:
      return t('app.move.check_other');
  }
}

/** A name taken is resolved by a new name that differs; every other obstacle only by the server. */
export function isObstacleResolved(finding: MoveFinding, newName: string, currentName: string): boolean {
  if (finding.code !== 'containers.name_taken') return false;
  const name = newName.trim();
  return name.length > 0 && name !== currentName;
}
