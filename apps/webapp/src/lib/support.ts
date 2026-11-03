// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a person hands to support from *Help* (user-experience.md §5.8, *Copy details*): the
 * product version, the workspace's short name, the page and the references of the last refusals
 * this tab saw - nothing anybody wrote. The page is its path only: a query or a fragment can hold
 * a search, which is content.
 */

const KEPT = 5;
let references: string[] = [];

/** Notes a refusal's reference; the newest first, each once, the last few only. */
export function rememberReference(id: string | undefined): void {
  if (!id) return;
  references = [id, ...references.filter((each) => each !== id)].slice(0, KEPT);
}

export function recentReferences(): readonly string[] {
  return references;
}

/** For a test: start again with none. */
export function forgetReferences(): void {
  references = [];
}

export interface SupportFacts {
  readonly version: string | undefined;
  readonly workspace: string | undefined;
  /** The location's path and anything after it; only the path is kept. */
  readonly route: string;
  readonly references: readonly string[];
  /** The words of each line, resolved by the caller (rule 8 holds for the client's modules too). */
  readonly labels: { readonly version: string; readonly workspace: string; readonly route: string; readonly references: string };
}

/** The details as lines of `label: value`, a dash where a value is not known. */
export function supportDetails(facts: SupportFacts): string {
  const path = facts.route.split(/[?#]/u)[0] || '/';
  return [
    `${facts.labels.version}: ${facts.version || '—'}`,
    `${facts.labels.workspace}: ${facts.workspace || '—'}`,
    `${facts.labels.route}: ${path}`,
    `${facts.labels.references}: ${facts.references.length > 0 ? facts.references.join(', ') : '—'}`,
  ].join('\n');
}
