<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A ticket's deadlines at the top of its rail (ET-6, UC-SVC-02 check 6): one row per deadline by
  // urgency, with bar and time; *All deadlines as a table* opens the matrix - one row per policy
  // (the requester's SLA, then each OLA by its group), one column per goal. The head's one chip is
  // handed back to the page.
  import { untrack } from 'svelte';
  import { DeadlineList, Dialog, StateLozenge } from '@hubtask/design-system/components';
  import type { Deadline, DeadlineChip } from '@hubtask/design-system/components';
  import type { EntrySlaGoal, EntrySlaList, SlaPolicyList } from '@hubtask/sync-engine';
  import { entrySlaPath, policiesPath } from '../data/desk.svelte.ts';
  import { groups } from '../data/groups.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { byUrgency, chipOf, shareOf, timeOf, urgencyOf } from './deadlines.ts';
  import { spanOf } from './sla.ts';

  let { itemId, chip = $bindable(undefined), count = $bindable(0) }: { itemId: string; chip?: DeadlineChip; count?: number } = $props();

  const read = resource<EntrySlaList>(untrack(() => entrySlaPath(itemId)));
  // A policy and a group by their names, never their keys (ET-6): any member may read both.
  const named = resource<SlaPolicyList>(policiesPath);
  $effect(() => untrack(() => groups.open()));
  const goals = $derived(read.state.status === 'ready' ? read.state.data.data : []);
  const now = Date.now();
  const at = (iso: string | null) => (iso ? formatDateTime(iso, messages.locale) : '');
  const goalName = (goal: EntrySlaGoal) => t(`app.portal.goal.${goal.goal}`);
  const policyName = (key: string) =>
    named.state.status === 'ready' ? named.state.data.data.find((policy) => policy.key === key)?.name : undefined;
  const whose = (goal: EntrySlaGoal) => {
    const name = goal.kind === 'OLA'
      ? ((goal.assignee_group ? groups.nameOf(goal.assignee_group) : undefined) ?? policyName(goal.policy_key))
      : policyName(goal.policy_key);
    return name ? `${name} · ${goal.kind}` : goal.kind;
  };
  const words = (goal: EntrySlaGoal) =>
    goal.standing === 'MET' ? t('app.portal.goal_met', { at: at(goal.met_at) })
    : goal.standing === 'BREACHED' ? t('app.portal.goal_breached', { at: at(goal.breached_at) })
    : goal.standing === 'PAUSED' ? t('app.desk.goal_paused')
    : t('app.portal.goal_due', { at: at(goal.due_at) });
  const spanText = (ms: number) => {
    const span = spanOf(ms);
    return t(span.code, span.params);
  };
  /** What is left, or how long it is overdue - the row's words; the moment stands under the bar. */
  const remaining = (goal: EntrySlaGoal) => {
    const time = timeOf(goal, now);
    if (!time) return goal.standing === 'PAUSED' ? t('app.desk.goal_paused') : t('app.desk_queues.sla_running');
    return t(time.isOverdue ? 'app.entry.deadline_overdue' : 'app.entry.deadline_left', { span: spanText(time.span) });
  };
  const dueLine = (goal: EntrySlaGoal) => {
    const time = timeOf(goal, now);
    const moment = goal.due_at ?? goal.breached_at;
    if (!time || !moment) return undefined;
    return t(time.isOverdue ? 'app.entry.deadline_was_due' : 'app.entry.deadline_due', { at: at(moment) });
  };
  const TONE = { breached: 'danger', atRisk: 'warning', running: 'neutral', paused: 'neutral', met: 'success' } as const;

  // A kept deadline asks nothing more of anyone: those fold into one line per goal (the scene's
  // *First reply: 3 of 3 kept*); the table still holds each.
  const rows = $derived<Deadline[]>(
    byUrgency(goals, now).filter((goal) => goal.standing !== 'MET').map((goal) => ({
      id: `${goal.kind}-${goal.policy_key}-${goal.goal}`,
      name: goalName(goal),
      context: whose(goal),
      words: remaining(goal),
      due: dueLine(goal),
      share: shareOf(goal, now),
      tone: TONE[urgencyOf(goal, now)],
      isPaused: goal.standing === 'PAUSED',
    })),
  );

  const keptLabel = $derived(
    [...new Set(goals.filter((goal) => goal.standing === 'MET').map((goal) => goal.goal))]
      .map((kind) => {
        const ofKind = goals.filter((goal) => goal.goal === kind);
        return t('app.entry.deadlines_kept', { goal: t(`app.portal.goal.${kind}`), kept: String(ofKind.filter((goal) => goal.standing === 'MET').length), count: String(ofKind.length) });
      })
      .join(' · ') || undefined,
  );

  $effect(() => {
    count = goals.length;
    const first = chipOf(goals, now);
    chip = first
      ? {
          label: first.isBreached
            ? t('app.entry.deadline_chip_breached', { goal: goalName(first.goal) })
            : t('app.entry.deadline_chip_next', { goal: goalName(first.goal), rest: remaining(first.goal) }),
          tone: first.isBreached ? 'danger' : urgencyOf(first.goal, now) === 'atRisk' ? 'warning' : 'neutral',
        }
      : undefined;
  });

  let isTableOpen = $state(false);
  const GOALS = ['FIRST_RESPONSE', 'NEXT_RESPONSE', 'RESOLUTION'] as const;
  const policies = $derived([...new Map(goals.map((goal) => [`${goal.kind}-${goal.policy_key}-${goal.assignee_group ?? ''}`, goal])).values()]);
  const cell = (policy: EntrySlaGoal, kind: string) =>
    goals.find((goal) => goal.kind === policy.kind && goal.policy_key === policy.policy_key && goal.assignee_group === policy.assignee_group && goal.goal === kind);
</script>

{#if goals.length > 0}
  <div data-entry-sla>
    <DeadlineList deadlines={rows} {keptLabel} table={{ label: t('app.entry.all_deadlines'), onclick: () => (isTableOpen = true) }} />
  </div>
{/if}

{#if isTableOpen}
  <Dialog size="l" isOpen={true} title={t('app.entry.all_deadlines')} dismissLabel={t('app.workflows.close')} onClose={() => (isTableOpen = false)}>
    <!-- The matrix as rows of a list: one per policy, its goals side by side (a dialog holds no table). -->
    <ul class="matrix" data-deadline-matrix>
      {#each policies as policy (`${policy.kind}-${policy.policy_key}-${policy.assignee_group}`)}
        <li class="policy">
          <span class="whose">{whose(policy)}</span>
          <dl class="goals">
            {#each GOALS as kind (kind)}
              {@const goal = cell(policy, kind)}
              {#if goal}
                <div><dt>{t(`app.portal.goal.${kind}`)}</dt><dd><StateLozenge label={words(goal)} vocabulary="sla" state={urgencyOf(goal, now)} size="sm" /></dd></div>
              {/if}
            {/each}
          </dl>
        </li>
      {/each}
    </ul>
  </Dialog>
{/if}

<style>
  .matrix { display: flex; flex-direction: column; margin: 0; padding: 0; list-style: none; font-size: var(--fs-075); }
  .policy { display: flex; flex-direction: column; gap: var(--sp-050); padding-block: var(--sp-100); border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  .whose { font-weight: var(--fw-medium); }
  .goals { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--sp-100); margin: 0; }
  .goals dt { color: var(--text-subtle); }
  .goals dd { margin: 0; }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) { .goals { grid-template-columns: minmax(0, 1fr); } }
</style>
