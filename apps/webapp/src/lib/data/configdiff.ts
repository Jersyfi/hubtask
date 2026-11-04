// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Two configuration documents compared block by block (configuration.md §5, *Changes*): each
 * block whose key path is in one and not the other, or whose attributes differ, in the order of
 * its section and key, so two versions are read side by side by what changed between them.
 */

import { changeWords } from './configtemplates.ts';

type Block = Record<string, unknown>;
type Document = Record<string, Record<string, Block | null>>;

export type BlockDifference = {
  /** `section.key`, as a plan and a refusal name it. */
  readonly path: string;
  readonly kind: 'ADDED' | 'CHANGED' | 'REMOVED';
  readonly before?: Block;
  readonly after?: Block;
};

/** Canonical JSON: keys sorted at every depth, so two equal blocks compare equal. */
const canonical = (value: unknown): string =>
  JSON.stringify(value, (_key, inner: unknown) =>
    inner && typeof inner === 'object' && !Array.isArray(inner)
      ? Object.fromEntries(Object.entries(inner as Record<string, unknown>).sort(([a], [b]) => a.localeCompare(b)))
      : inner,
  );

/** What changed from `before` to `after`, block by block. */
export function compareDocuments(before: Document | undefined, after: Document | undefined): BlockDifference[] {
  const from = before ?? {};
  const to = after ?? {};
  const sections = [...new Set([...Object.keys(from), ...Object.keys(to)])].sort();
  const out: BlockDifference[] = [];
  for (const section of sections) {
    const a = from[section] ?? {};
    const b = to[section] ?? {};
    const keys = [...new Set([...Object.keys(a), ...Object.keys(b)])].sort();
    for (const key of keys) {
      const was = a[key] ?? undefined;
      const is = b[key] ?? undefined;
      const path = `${section}.${key}`;
      if (was === undefined && is !== undefined) out.push({ path, kind: 'ADDED', after: is });
      else if (was !== undefined && is === undefined) out.push({ path, kind: 'REMOVED', before: was });
      else if (was !== undefined && is !== undefined && canonical(was) !== canonical(is)) {
        out.push({ path, kind: 'CHANGED', before: was, after: is });
      }
    }
  }
  return out;
}

const OPS = { ADDED: 'ADD', CHANGED: 'CHANGE', REMOVED: 'REMOVE' } as const;

/**
 * A difference in words (user-experience.md §8.6): what happened to which part, by its name where
 * it has one - the same phrase a plan's change reads as. The path stays for the expert's fold.
 */
export function differenceWords(difference: BlockDifference): { code: string; section: string; key: string; module?: string } {
  const dot = difference.path.indexOf('.');
  const section = dot < 0 ? difference.path : difference.path.slice(0, dot);
  const key = dot < 0 ? '' : difference.path.slice(dot + 1);
  return changeWords({ section, key, op: OPS[difference.kind], before: difference.before, after: difference.after });
}
