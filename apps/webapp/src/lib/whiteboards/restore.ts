// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Bringing a board's older version back (UC-CNV-06 check 3): what stands is saved and, when the
 * newest version holding it has no name, named - a named version is never folded into, so the
 * save after the restore cannot overwrite it in the history. Then the scene is replaced.
 */

/** A version as the list answers it: newest first. */
export interface Listed {
  readonly seq: number;
  readonly name?: string | null;
}

export interface RestoreSteps {
  /** Saves the scene as it stands. */
  save(): Promise<void>;
  versions(): Promise<readonly Listed[]>;
  name(seq: number, name: string): Promise<unknown>;
  /** Puts the version's objects in place of the scene; false when nothing changed. */
  replace(): boolean;
}

/** Answers whether the board now shows the version brought back. */
export async function keepThenReplace(steps: RestoreSteps, seq: number, keptName: string): Promise<boolean> {
  await steps.save();
  const newest = (await steps.versions())[0];
  if (newest && newest.seq !== seq && !newest.name) await steps.name(newest.seq, keptName);
  if (!steps.replace()) return false;
  await steps.save();
  return true;
}
