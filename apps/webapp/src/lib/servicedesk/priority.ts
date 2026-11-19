// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Whether an entry's priority is the one its impact and urgency give (UC-SVC-02 check 14). The
// two rules that set it are the desk's and only an administrator may read rules, so a handler's
// client cannot ask them; it reads the values instead, by the ITIL template's matrix. A priority
// set by hand to another value says nothing of where it came from.

const LEVEL: Readonly<Record<string, number>> = { High: 0, Medium: 1, Low: 2 };

/** The template's matrix: high × high P1; one high and one medium P2; a sum of two P3; the rest P4. */
function matrixPriority(impact: number, urgency: number): string {
  const sum = impact + urgency;
  return sum === 0 ? 'P1' : sum === 1 ? 'P2' : sum === 2 ? 'P3' : 'P4';
}

export function followsMatrix(fields: Readonly<Record<string, unknown>> | undefined): boolean {
  const impact = LEVEL[String(fields?.impact)];
  const urgency = LEVEL[String(fields?.urgency)];
  if (impact === undefined || urgency === undefined) return false;
  return fields?.priority === matrixPriority(impact, urgency);
}
