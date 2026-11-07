// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Where a value set above the reader comes from, in the words of P-07 (UC-WRK-32 check 5).
 *
 * Settings resolve Hubtask's minimum → installation → plan → workspace, and the server says, per
 * value, which level decided it and whether a level locked it. This maps that answer to the
 * message codes `SourceChip` draws, so every screen says it in the same words: "Set by the
 * installation", "Set by your plan", "Set here", "Hubtask's default". Nothing is re-derived here.
 * A value whose answer carries no source, as from an older server, gets no chip at all rather
 * than a guess.
 *
 * The codes only: the caller resolves them (rule 8), and this stays testable without a renderer.
 */

/** The levels a value can come from, as the contract names them across its resources. */
export type ValueSource = 'DEFAULT' | 'INSTANCE' | 'PLAN' | 'WORKSPACE' | 'HUB' | 'EXCEPTION';

/** Which level locked a value, where one did. */
export type ValueLock = 'INSTANCE' | 'PLAN';

/** The chip's words, as message codes. */
export interface ChipCodes {
  /** The chip's own text, which is also its accessible name. */
  readonly label: string;
  readonly isLocked: boolean;
  /** The source in a sentence, for the opened chip. */
  readonly source: string;
  /** Whom to ask, where somebody else decides. */
  readonly whomToAsk?: string;
}

const KNOWN: ReadonlySet<string> = new Set(['DEFAULT', 'INSTANCE', 'PLAN', 'WORKSPACE', 'HUB', 'EXCEPTION']);

/**
 * The chip for one value, or `undefined` where the server named no source.
 *
 * A lock names the level that holds it, which is the level the value comes from: a value the
 * installation locked is the installation's, whatever the workspace once stored. `operates` is
 * whether the reader runs this installation themselves - in a private installation they usually
 * do, and "ask the operator" would send them to themselves. `atHub` is whether the value is read
 * at a hub, where the hub's own is "set here" and the workspace's is a level above.
 */
export function chipCodes(
  origin: { readonly source?: string | null; readonly lock?: string | null },
  reader: { readonly operates: boolean; readonly atHub?: boolean },
): ChipCodes | undefined {
  const lock = origin.lock === 'INSTANCE' || origin.lock === 'PLAN' ? origin.lock : undefined;
  const source = lock ?? origin.source;
  if (!source || !KNOWN.has(source)) return undefined;
  const isLocked = lock !== undefined;
  const level = reader.atHub && source === 'WORKSPACE' ? 'workspace_above' : source.toLowerCase();
  return {
    label: `app.source.${isLocked ? 'locked' : 'label'}_${level}`,
    isLocked,
    source: `app.source.${isLocked ? 'locked_sentence' : 'sentence'}_${level}`,
    whomToAsk: whomToAsk(source as ValueSource, isLocked, reader.operates),
  };
}

/**
 * Whom to ask, only where the reader cannot change the value here: a lock, or an operator's
 * exception. An inherited value nobody locked is changed on this very screen.
 */
function whomToAsk(source: ValueSource, isLocked: boolean, operates: boolean): string | undefined {
  if (!isLocked && source !== 'EXCEPTION') return undefined;
  if (source === 'PLAN') return operates ? 'app.source.ask_yourself_plan' : 'app.source.ask_provider';
  return operates ? 'app.source.ask_yourself' : 'app.source.ask_operator';
}
