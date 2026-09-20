// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the check found, as words and as marks on the canvas (F8-07, ADR-0060). A finding's
 * `path` is the JSON pointer a refusal's field errors already use, so the same translation puts
 * both at the card they are about.
 */

import type { RuleFinding } from '../data/rules.svelte.ts';
import { pathOf } from './model.ts';
import type { Catalogue } from './words.ts';

/** The card a finding is about: `trigger`, `run_as`, `conditions/1`, or a step's path. */
export function cardOf(pointer: string): string | undefined {
  if (pointer.startsWith('/trigger')) return 'trigger';
  if (pointer === '/run_as') return 'run_as';
  const condition = /^\/conditions\/(\d+)/.exec(pointer);
  if (condition) return `conditions/${condition[1]}`;
  return pathOf(pointer);
}

/** The finding as a sentence: its code and parameters, or the code where the catalogue has no entry. */
export function findingWords(words: Catalogue, finding: RuleFinding): string {
  return words.has(finding.code) ? words.t(finding.code, finding.params) : finding.code;
}

/** Every finding at the card it is about, the first per card winning: what the canvas marks. */
export function marksOf(words: Catalogue, findings: readonly RuleFinding[]): Map<string, string> {
  const marks = new Map<string, string>();
  for (const finding of findings) {
    const card = cardOf(finding.path);
    if (card && !marks.has(card)) marks.set(card, findingWords(words, finding));
  }
  return marks;
}

/** The health of a rule as the list sees it: from its findings and its switch alone, without a page of runs per card. */
export function listHealth(rule: { enabled: boolean; findings?: readonly RuleFinding[]; failure_count: number }): 'works' | 'sometimes' | 'off' | 'attention' | 'broken' {
  const findings = rule.findings ?? [];
  if (findings.some((finding) => finding.level === 'BROKEN')) return 'broken';
  if (!rule.enabled) return 'off';
  if (findings.length > 0) return 'attention';
  return rule.failure_count > 0 ? 'sometimes' : 'works';
}
