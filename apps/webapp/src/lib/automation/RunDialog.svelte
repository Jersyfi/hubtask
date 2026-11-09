<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One run of a rule as a Dialog M (page-system §7.6): how it ended and why, in words, what each
  // of its actions did, and - for a failed run - *Finish this run*. The keys - the run's and the
  // rule's identifiers, each action's kind and path, the failure codes - are under *Details for
  // support* (PS-15).
  //
  // **This is where the seven statuses earn their distinctness.** `SKIPPED` names *which* condition
  // stopped the run - the contract puts the condition results in the rule's own order precisely so
  // that this question has an answer. `THROTTLED` says the rule protected the workspace from
  // itself. `ABORTED_LOOP` says the causation depth stopped a rule that triggered itself. And
  // `RUNNING` on a row that started long ago is a crash rather than progress.
  //
  // **A replay completes a failed run rather than starting a new one.** The actions that already
  // succeeded are named before the button, because confusing a replay with a re-trigger is how
  // somebody sends the same mail twice.
  import { Badge, Banner, Button, Dialog, Disclosure, RunStatusBadge, Stack } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { runs, type ActionResult, type Run } from '../data/runs.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { isStale, statusWord, stoppedBy, triggerWord } from './runwords.ts';

  interface Props {
    /** The run opened; closed while undefined. */
    run: Run | undefined;
    ruleName: (id: string) => string;
    /** Whether the reader may finish a failed run: the automation permission, the rules in the plan. */
    mayReplay: boolean;
    onClose: () => void;
  }

  const { run, ruleName, mayReplay, onClose }: Props = $props();

  $effect(() => {
    if (run) void runs.read(run.id);
  });

  const detail = $derived(run ? (runs.detail(run.id) ?? run) : undefined);
  const said = (code: string | null | undefined) => (code ? (messages.has(code) ? t(code) : undefined) : undefined);
  const when = (at: string | null | undefined) => (at ? formatDateTime(at, messages.locale) : '');
  const alreadyDone = (each: Run) => each.action_results.filter((result) => result.status === 'SUCCEEDED');
  const tone = (result: ActionResult) => (result.status === 'SUCCEEDED' ? 'success' : result.status === 'FAILED' ? 'danger' : 'neutral');

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  async function replay(): Promise<void> {
    if (!detail) return;
    failure = undefined;
    isWorking = true;
    try {
      await runs.replay(detail.id);
      announcer.say(t('app.runs.replayed_announced'));
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }

  const isReplayable = $derived(detail?.status === 'FAILED' && mayReplay);
</script>

<Dialog
  size="m"
  isOpen={run !== undefined}
  title={run ? ruleName(run.rule_id) : ''}
  dismissLabel={t('app.page.close')}
  onClose={() => { failure = undefined; onClose(); }}
  primary={isReplayable ? { label: t('app.runs.replay'), isBusy: isWorking, busyLabel: t('app.runs.replaying'), onclick: () => void replay() } : undefined}
>
  {#if detail}
    <Stack gap="200">
      {#if failure}<FailureNote problem={failure} />{/if}
      <dl class="facts" data-run-detail={detail.id}>
        <div class="fact">
          <dt>{t('app.runs.outcome')}</dt>
          <dd>
            <RunStatusBadge
              status={detail.status}
              label={isStale(detail) ? t('app.runs.status_crashed') : statusWord(detail.status)}
              isDryRun={detail.is_dry_run ?? false}
              dryRunLabel={t('app.runs.dry_run')}
            />
          </dd>
        </div>
        <div class="fact"><dt>{t('app.runs.when')}</dt><dd>{when(detail.started_at)}</dd></div>
        <div class="fact">
          <dt>{t('app.runs.filter_trigger')}</dt>
          <dd>
            {triggerWord(detail.trigger)}
            {#if detail.triggered_by}· {t('app.runs.pulled_by', { name: accounts.nameOf(detail.triggered_by) ?? t('app.people.unnamed') })}{/if}
          </dd>
        </div>
      </dl>

      {#if detail.status === 'SKIPPED' && stoppedBy(detail) !== undefined}
        <p class="quiet">{t('app.runs.skipped_by', { index: String((stoppedBy(detail) ?? 0) + 1) })}</p>
      {:else if detail.status === 'THROTTLED'}
        <p class="quiet">{t('app.runs.throttled_note')}</p>
      {:else if detail.status === 'ABORTED_LOOP'}
        <p class="quiet">{t('app.runs.aborted_note', { depth: String(detail.causation_depth) })}</p>
      {:else if isStale(detail)}
        <p class="quiet">{t('app.runs.crashed_note')}</p>
      {/if}
      {#if said(detail.error_code)}<p class="failure" role="alert">{said(detail.error_code)}</p>{/if}

      {#if detail.action_results.length > 0}
        <ol class="actions">
          {#each detail.action_results as result (result.index)}
            <li>
              <span>{t('app.runs.action_n', { n: String(result.index + 1) })}</span>
              <Badge tone={tone(result)}>{statusWord(result.status)}</Badge>
              {#if result.matched !== undefined}
                <span class="quiet">{result.matched ? t('app.runs.branch_then_ran') : t('app.runs.branch_else_ran')}</span>
              {/if}
              {#if said(result.error_code)}<span class="failure">{said(result.error_code)}</span>{/if}
            </li>
          {/each}
        </ol>
      {/if}

      {#if isReplayable}
        <Banner tone="info">{t('app.runs.replay_note', { count: String(alreadyDone(detail).length) })}</Banner>
      {/if}

      <Disclosure label={t('app.runs.support')} tone="quiet">
        <dl class="support">
          <dt>{t('app.runs.support_run')}</dt><dd class="mono">{detail.id}</dd>
          <dt>{t('app.runs.support_rule')}</dt><dd class="mono">{detail.rule_id}</dd>
          <dt>{t('app.runs.support_depth')}</dt><dd class="mono">{detail.causation_depth}</dd>
          {#if detail.error_code}<dt>{t('app.runs.support_error')}</dt><dd class="mono">{detail.error_code}</dd>{/if}
          {#each detail.action_results as result (result.index)}
            <dt>{t('app.runs.action_n', { n: String(result.index + 1) })}</dt>
            <dd class="mono">{result.path ? `${result.kind} · ${result.path}` : result.kind}{result.error_code ? ` · ${result.error_code}` : ''}</dd>
          {/each}
        </dl>
      </Disclosure>
    </Stack>
  {/if}
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.page.close')}</Button>
  {/snippet}
</Dialog>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .failure { margin: 0; color: var(--text-danger); }

  .facts { margin: 0; display: grid; gap: var(--sp-100); }
  .fact { display: grid; grid-template-columns: minmax(0, 12ch) minmax(0, 1fr); gap: var(--sp-150); align-items: center; }
  .fact dt { color: var(--text-secondary); }
  .fact dd { margin: 0; }

  .actions { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .actions li { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .support { margin: 0; display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: var(--sp-050) var(--sp-150); font-size: var(--fs-075); }
  .support dt { color: var(--text-secondary); }
  .support dd { margin: 0; }
  .mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
</style>
