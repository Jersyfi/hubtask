// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/** What is selected on the canvas, and therefore what the inspector shows (F8-04). */
export type Selection =
  | { kind: 'rule' }
  | { kind: 'trigger' }
  | { kind: 'scope' }
  | { kind: 'runas' }
  | { kind: 'gate' }
  | { kind: 'condition'; index: number }
  | { kind: 'step'; path: string }
  | { kind: 'guardrails' };
