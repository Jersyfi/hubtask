// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A run drawn onto the canvas (F8-06, `milestone-F8.md` decisions 5 and 9): what a dry run or a
 * recorded run says about each card, in the order the run visits them, and the health a rule has
 * from its last runs. Pure, so that the arithmetic and the order are held by tests.
 */

import type { ConditionResult, Run, TestAction, TestResult } from '../data/runs.svelte.ts';
import type { Step } from './model.ts';
import { walk } from './model.ts';

/** What one card says once the run has reached it. */
export interface Verdict {
  /** `yes` held or would run; `no` did not hold, was refused, or the arm not taken; `skipped` never reached. */
  readonly state: 'yes' | 'no' | 'skipped';
  /** A message code for the badge on the card. */
  readonly code: string;
  readonly params?: Record<string, string | number>;
}

/** One frame of the drawing: which card, and what it says. `trigger`, `conditions/1`, or a step's path. */
export interface Frame {
  readonly key: string;
  readonly verdict: Verdict;
}

/** The outcome, once every frame is drawn. */
export interface Outcome {
  readonly status: string;
  readonly code: string;
  readonly params?: Record<string, string | number>;
}

function conditionFrames(results: readonly ConditionResult[]): { frames: Frame[]; held: boolean } {
  const frames: Frame[] = [];
  let held = true;
  for (const result of results) {
    const ok = result.matched && !result.error_code;
    held = held && ok;
    frames.push({
      key: `conditions/${result.index}`,
      verdict: ok
        ? { state: 'yes', code: 'app.flow.verdict_held' }
        : { state: 'no', code: result.error_code ? 'app.flow.verdict_unreadable' : 'app.flow.verdict_not_held' },
    });
  }
  return { frames, held };
}

/**
 * The frames of a dry run, in the order the run would visit the cards: the trigger, every
 * condition, then every action the result names - both arms of a branch, the arm not taken drawn
 * as skipped rather than left blank, because "and what if it had not" is what the dry run answers.
 */
export function framesOfTest(actions: readonly Step[], result: TestResult): { frames: Frame[]; outcome: Outcome } {
  const frames: Frame[] = [{ key: 'trigger', verdict: { state: 'yes', code: 'app.flow.verdict_fires' } }];
  const conditions = conditionFrames(result.condition_results);
  frames.push(...conditions.frames);
  const byPath = new Map<string, TestAction>();
  for (const action of result.actions) byPath.set(action.path, action);

  let wouldRun = 0;
  walk(actions, (step, path) => {
    const planned = byPath.get(path);
    if (!planned) return;
    if (step.kind === 'BRANCH') {
      const matched = (planned as { matched?: boolean }).matched;
      frames.push({
        key: path,
        verdict: !planned.would_run
          ? { state: 'skipped', code: 'app.flow.verdict_skipped' }
          : matched
            ? { state: 'yes', code: 'app.flow.verdict_then' }
            : { state: 'no', code: 'app.flow.verdict_otherwise' },
      });
      return;
    }
    if (planned.would_run) wouldRun += 1;
    frames.push({
      key: path,
      verdict: planned.would_run
        ? { state: 'yes', code: step.kind === 'STOP' ? 'app.flow.verdict_ends' : step.kind === 'WAIT' ? 'app.flow.verdict_parks' : 'app.flow.verdict_would_run' }
        : { state: 'skipped', code: 'app.flow.verdict_skipped' },
    });
  });

  const outcome: Outcome = !result.matched
    ? { status: 'SKIPPED', code: 'app.flow.outcome_skipped' }
    : { status: 'SUCCEEDED', code: 'app.flow.outcome_would_run', params: { count: wouldRun } };
  return { frames, outcome };
}

/** The frames of a recorded run: what each card did, from the log's own results. */
export function framesOfRun(actions: readonly Step[], run: Run): { frames: Frame[]; outcome: Outcome } {
  const frames: Frame[] = [{ key: 'trigger', verdict: { state: 'yes', code: 'app.flow.verdict_fired' } }];
  frames.push(...conditionFrames(run.condition_results).frames);
  const byPath = new Map<string, Run['action_results'][number]>();
  run.action_results.forEach((result, at) => byPath.set(result.path ?? String(result.index ?? at), result));
  walk(actions, (step, path) => {
    const done = byPath.get(path);
    if (!done) {
      if (run.status !== 'SKIPPED' && run.status !== 'THROTTLED') frames.push({ key: path, verdict: { state: 'skipped', code: 'app.flow.verdict_not_reached' } });
      return;
    }
    if (step.kind === 'BRANCH') {
      frames.push({ key: path, verdict: done.matched === false ? { state: 'no', code: 'app.flow.verdict_otherwise' } : { state: 'yes', code: 'app.flow.verdict_then' } });
      return;
    }
    frames.push({
      key: path,
      verdict:
        done.status === 'SUCCEEDED'
          ? { state: 'yes', code: step.kind === 'STOP' ? 'app.flow.verdict_ended' : 'app.flow.verdict_ran' }
          : done.status === 'FAILED'
            ? { state: 'no', code: 'app.flow.verdict_failed' }
            : { state: 'skipped', code: 'app.flow.verdict_skipped' },
    });
  });
  return { frames, outcome: { status: run.status, code: `app.runs.status_${run.status.toLowerCase()}` } };
}

/* ---------- Health ---------- */

export type Health = 'works' | 'sometimes' | 'failing' | 'off' | 'attention' | 'broken' | 'unknown';

/**
 * The client's arithmetic over the last page of runs (decision 5): off beats everything but a
 * finding; a BROKEN finding is broken and an ATTENTION one needs attention; then the failures
 * among the runs - none is works, more than half is failing, any is sometimes; and a rule with
 * no run yet is unknown rather than pretended healthy.
 */
export function healthOf(input: {
  enabled: boolean;
  findings: readonly { level: string }[];
  runs: readonly { status: string }[];
}): Health {
  if (input.findings.some((finding) => finding.level === 'BROKEN')) return 'broken';
  if (!input.enabled) return 'off';
  if (input.findings.length > 0) return 'attention';
  const counted = input.runs.filter((run) => run.status !== 'RUNNING' && run.status !== 'WAITING');
  if (counted.length === 0) return 'unknown';
  const failed = counted.filter((run) => run.status === 'FAILED' || run.status === 'ABORTED_LOOP').length;
  if (failed === 0) return 'works';
  return failed * 2 > counted.length ? 'failing' : 'sometimes';
}
