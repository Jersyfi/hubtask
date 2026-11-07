// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The administration overview's first steps (user-experience.md §8.5): what this workspace still
 * needs, only the steps this reader may act on - the permission the step's screen writes with -
 * each ticked by the state it checks.
 *
 * A fact still being read is `undefined`, and the card waits for it rather than drawing a step
 * open that turns done a moment later. A read that failed or was refused is `null`: its step is
 * left out, because a step that can never be ticked would stay open for good.
 */

export type StepId = 'invite' | 'products' | 'mailbox' | 'signin' | 'backup' | 'support';

/** One fact: `true`/`false` once read, `undefined` while reading, `null` when it cannot be read. */
export type Fact = boolean | null | undefined;

export interface FirstStepFacts {
  /** The permissions this reader holds at the workspace. */
  readonly holds: (permission: string) => boolean;
  /** Whether the service desk is on - the mailbox step is the desk's. */
  readonly isDeskOn: boolean;
  readonly hasSeveralPeople: Fact;
  readonly hasModuleOn: Fact;
  readonly hasMailbox: Fact;
  readonly hasSignIn: Fact;
  readonly hasNightlyBackup: Fact;
  /** Whether the workspace names a backup target yet: without one the step says that no backup
   * is made at all, since Hubtask creates no target or schedule by itself. */
  readonly hasBackupTarget: Fact;
  /** Whether this workspace may add a backup target of its own (the manifest's `backup_targets`):
   * where the operator keeps the backups, the step could never be ticked. */
  readonly mayAddBackupTarget: boolean;
  readonly hasSupportContact: Fact;
}

export interface FirstStep {
  readonly id: StepId;
  readonly isDone: boolean;
  /** Where the step is done. */
  readonly page: string;
  /** The message key's suffix the step is said with: its id, or a variant of it. */
  readonly label: string;
}

export interface FirstSteps {
  /** `false` while a fact a shown step needs is still out. */
  readonly isSettled: boolean;
  readonly steps: readonly FirstStep[];
  readonly done: number;
}

const STEPS: readonly {
  readonly id: StepId;
  readonly permission: string;
  readonly page: string;
  readonly fact: (facts: FirstStepFacts) => Fact;
  readonly applies?: (facts: FirstStepFacts) => boolean;
  readonly label?: (facts: FirstStepFacts) => string;
}[] = [
  { id: 'invite', permission: 'MANAGE_MEMBERS', page: '/administration/people', fact: (facts) => facts.hasSeveralPeople },
  { id: 'products', permission: 'STRUCTURE', page: '/administration/modules', fact: (facts) => facts.hasModuleOn },
  { id: 'mailbox', permission: 'AUTOMATION', page: '/administration/mail', fact: (facts) => facts.hasMailbox, applies: (facts) => facts.isDeskOn },
  { id: 'signin', permission: 'STRUCTURE', page: '/administration/sign-in', fact: (facts) => facts.hasSignIn },
  { id: 'backup', permission: 'DELETE_CONTAINER', page: '/administration/backup', fact: (facts) => facts.hasNightlyBackup, applies: (facts) => facts.mayAddBackupTarget,
    label: (facts) => (facts.hasBackupTarget === false ? 'backup_none' : 'backup') },
  { id: 'support', permission: 'STRUCTURE', page: '/administration/legal', fact: (facts) => facts.hasSupportContact },
];

export function firstSteps(facts: FirstStepFacts): FirstSteps {
  const mine = STEPS.filter((step) => facts.holds(step.permission) && (step.applies?.(facts) ?? true));
  const read = mine.map((step) => ({ step, fact: step.fact(facts) }));
  const isSettled = read.every(({ fact }) => fact !== undefined);
  const steps = read
    .filter(({ fact }) => fact !== null && fact !== undefined)
    .map(({ step, fact }) => ({ id: step.id, isDone: fact === true, page: step.page, label: step.label?.(facts) ?? step.id }));
  return { isSettled, steps, done: steps.filter((step) => step.isDone).length };
}

/** Whether the card is drawn: settled, something to show, and not all of it done. */
export const isOwed = (steps: FirstSteps): boolean => steps.isSettled && steps.steps.some((step) => !step.isDone);
