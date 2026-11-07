// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Who a configuration version is by, as the history says it (configuration.md §5): a person, an
// agent, a change made on the screens - or, a DIRECT version that changed nothing, the state a
// scope had before its first apply, which a rollback returns to.

type Version = { actor_kind: string; added: number; changed: number; removed: number };

export function versionByCode(version: Version): string {
  if (version.actor_kind === 'AI_AGENT') return 'app.changes.by_agent';
  if (version.actor_kind !== 'DIRECT') return 'app.changes.by_person';
  return version.added + version.changed + version.removed === 0 ? 'app.changes.by_baseline' : 'app.changes.by_direct';
}
