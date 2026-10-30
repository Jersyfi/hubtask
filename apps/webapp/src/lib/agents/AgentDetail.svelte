<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One agent opened from *Agents*: what it spent, its ledger, its jobs with *Run on…* and its
  // evaluation set with the runs (UC-AGT-02 check 9, UC-AGT-04 check 6, UC-AGT-07 check 7). The
  // ledger never holds an input or an answer, so neither does this.

  import { untrack } from 'svelte';
  import {
    Badge, Banner, Button, Dialog, IconButton, Input, Select, Spinner, Stack, Table, Tabs, Textarea, type SelectOption,
  } from '@hubtask/design-system/components';
  import type { Agent, AgentActionList, AgentCaseList, AgentJobList, AgentRunList, WorkItem } from '@hubtask/sync-engine';

  import EntryPicker from '../entries/EntryPicker.svelte';
  import {
    agentActionsPath, agentCasesPath, agentEvaluationsPath, agentJobsPath, agentPath, agents, expectedFrom, jobTarget, mayStartJob, type Viewer,
  } from '../data/agents.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    agent: Agent;
    viewer: Viewer;
  }

  const { agent, viewer }: Props = $props();

  // The detail is keyed on the agent by its caller, so each path is fixed for its life.
  const id = untrack(() => agent.id);
  const one = resource<Agent>(agentPath(id));
  const ledger = resource<AgentActionList>(agentActionsPath(id));
  const jobs = resource<AgentJobList>(agentJobsPath(id));
  const cases = resource<AgentCaseList>(agentCasesPath(id));
  const runs = resource<AgentRunList>(agentEvaluationsPath(id));

  const usage = $derived(one.state.status === 'ready' ? one.state.data.usage : undefined);
  const target = $derived(jobTarget(agent.kind));
  const mayRun = $derived(mayStartJob(agent, viewer));
  const at = (iso: string | null | undefined) => (iso ? formatDateTime(iso, messages.locale) : '—');
  const failureOf = (state: { status: string; error?: unknown }) =>
    state.status === 'failed' ? renderProblem(state.error as never, messages).message : undefined;

  let shown = $state('ledger');
  const tabs = $derived([
    { id: 'ledger', label: t('app.agents.ledger') },
    ...(target ? [{ id: 'jobs', label: t('app.agents.jobs') }] : []),
    { id: 'evaluation', label: t('app.agents.evaluation') },
  ]);

  async function act<T>(run: () => Promise<T>, done: string): Promise<T | undefined> {
    try {
      const out = await run();
      toast.show({ text: done, tone: 'success' });
      return out;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
      return undefined;
    }
  }

  // --- Run on… ---------------------------------------------------------------------------------

  let isRunOpen = $state(false);
  let chosenEntry = $state<WorkItem | undefined>(undefined);
  let chosenHub = $state('');
  const hubOptions = $derived<SelectOption[]>(containers.hubs.map((each) => ({ value: each.id, label: each.name })));
  const runTarget = $derived(target === 'HUB' ? chosenHub : chosenEntry?.id);

  async function run() {
    if (!runTarget) return;
    const done = await act(() => agents.startJob(id, runTarget), t('app.agents.job_started'));
    if (done) {
      isRunOpen = false;
      chosenEntry = undefined;
      chosenHub = '';
      shown = 'jobs';
    }
  }

  // --- A case ----------------------------------------------------------------------------------

  let isCaseOpen = $state(false);
  let caseTitle = $state('');
  let caseNotes = $state('');
  let caseExpected = $state('{}');
  const parsed = $derived(expectedFrom(caseExpected));
  const caseReason = $derived(
    !caseTitle.trim() ? t('app.agents.case_needs_title')
    : 'problem' in parsed ? t(`app.agents.expected_${parsed.problem}`)
    : undefined,
  );

  async function addCase() {
    if (!('value' in parsed)) return;
    const done = await act(
      () => agents.addCase(id, { title: caseTitle.trim(), notes: caseNotes, expected: parsed.value }),
      t('app.agents.case_added'),
    );
    if (done) {
      isCaseOpen = false;
      caseTitle = '';
      caseNotes = '';
      caseExpected = '{}';
    }
  }
</script>

<Stack gap="200">
  {#if usage}
    <p class="meta">{t('app.agents.usage', { actions: usage.actions_this_hour ?? 0, tokens: usage.tokens_today ?? 0 })}</p>
  {/if}

  <Tabs label={agent.name} {tabs} bind:selected={shown}>
    {#if shown === 'ledger'}
      {#if failureOf(ledger.state)}
        <Banner tone="danger">{failureOf(ledger.state)}</Banner>
      {:else if ledger.state.status !== 'ready'}
        <Spinner label={t('app.agents.reading')} />
      {:else}
        <Table
          label={t('app.agents.ledger')}
          isLabelHidden
          columns={[
            { id: 'at', label: t('app.agents.col_at') },
            { id: 'use_case', label: t('app.agents.col_use_case') },
            { id: 'class', label: t('app.agents.col_class') },
            { id: 'decision', label: t('app.agents.col_decision') },
            { id: 'tokens', label: t('app.agents.col_tokens'), align: 'end' },
          ]}
        >
          {#each ledger.state.data.data as row (row.id)}
            <tr>
              <td>{at(row.at)}</td>
              <td><code>{row.use_case}</code></td>
              <td>{t(`app.agents.class.${row.class}`)}</td>
              <td>
                <Badge size="sm" tone={row.decision === 'REFUSED' ? 'danger' : row.decision === 'HELD' ? 'warning' : 'success'}>{t(`app.agents.decision.${row.decision}`)}</Badge>
                {#if row.code}<span class="meta">{messages.has(row.code) ? t(row.code) : row.code}</span>{/if}
              </td>
              <td class="number">{row.tokens}</td>
            </tr>
          {/each}
          {#snippet empty()}{t('app.agents.ledger_empty')}{/snippet}
        </Table>
      {/if}
    {:else if shown === 'jobs'}
      <Stack gap="100">
        {#if mayRun}
          <div><Button icon="play" onclick={() => (isRunOpen = true)}>{t('app.agents.run_on')}</Button></div>
        {:else if agent.status !== 'ACTIVE'}
          <p class="meta">{t('app.agents.runs_when_live')}</p>
        {/if}
        {#if failureOf(jobs.state)}
          <Banner tone="danger">{failureOf(jobs.state)}</Banner>
        {:else if jobs.state.status !== 'ready'}
          <Spinner label={t('app.agents.reading')} />
        {:else}
          <Table
            label={t('app.agents.jobs')}
            isLabelHidden
            columns={[
              { id: 'started', label: t('app.agents.col_started') },
              { id: 'target', label: t('app.agents.col_target') },
              { id: 'status', label: t('app.agents.col_status') },
              { id: 'proposals', label: t('app.agents.col_proposals'), align: 'end' },
              { id: 'tokens', label: t('app.agents.col_tokens'), align: 'end' },
            ]}
          >
            {#each jobs.state.data.data as job (job.id)}
              <tr>
                <td>{at(job.started_at)}</td>
                <td>
                  {#if job.target_type === 'HUB'}
                    {containers.find(job.target_id)?.name ?? t('app.agents.hub_unread')}
                  {:else}
                    <a href={`/items/${job.target_id}`}>{t('app.agents.open_entry')}</a>
                  {/if}
                </td>
                <td>
                  <Badge size="sm" tone={job.status === 'FAILED' ? 'danger' : job.status === 'RUNNING' ? 'info' : 'success'}>{t(`app.agents.job_status.${job.status}`)}</Badge>
                  {#if job.code}<span class="meta">{messages.has(job.code) ? t(job.code) : job.code}</span>{/if}
                </td>
                <td class="number">{job.proposals}</td>
                <td class="number">{job.tokens}</td>
              </tr>
            {/each}
            {#snippet empty()}{t('app.agents.jobs_empty')}{/snippet}
          </Table>
        {/if}
      </Stack>
    {:else}
      <Stack gap="150">
        <p class="meta">
          {agent.min_accuracy > 0 ? t('app.agents.bar', { min_accuracy: agent.min_accuracy }) : t('app.agents.no_bar')}
        </p>
        <h3 class="section">{t('app.agents.cases')}</h3>
        {#if viewer.administers}
          <div><Button icon="plus" onclick={() => (isCaseOpen = true)}>{t('app.agents.add_case')}</Button></div>
        {/if}
        {#if failureOf(cases.state)}
          <Banner tone="danger">{failureOf(cases.state)}</Banner>
        {:else if cases.state.status === 'ready'}
          <ul class="rows">
            {#each cases.state.data.data as each (each.id)}
              <li class="row">
                <span class="what">
                  <strong>{each.title}</strong>
                  <code class="meta">{JSON.stringify(each.expected)}</code>
                </span>
                {#if viewer.administers}
                  <IconButton
                    icon="trash"
                    size="sm"
                    tone="subtle"
                    label={t('app.agents.remove_case', { title: each.title })}
                    onclick={() => void act(() => agents.removeCase(id, each.id), t('app.agents.case_removed'))}
                  />
                {/if}
              </li>
            {:else}
              <li class="meta">{t('app.agents.cases_empty')}</li>
            {/each}
          </ul>
        {/if}

        <h3 class="section">{t('app.agents.runs')}</h3>
        {#if runs.state.status === 'ready'}
          <Table
            label={t('app.agents.runs')}
            isLabelHidden
            columns={[
              { id: 'at', label: t('app.agents.col_at') },
              { id: 'revision', label: t('app.agents.col_revision'), align: 'end' },
              { id: 'passed', label: t('app.agents.col_passed'), align: 'end' },
              { id: 'accuracy', label: t('app.agents.col_accuracy'), align: 'end' },
            ]}
          >
            {#each runs.state.data.data as each (each.id)}
              <tr>
                <td>{at(each.at)}</td>
                <td class="number">{each.model_revision}</td>
                <td class="number">{t('app.agents.passed_of', { passed: each.passed, cases: each.cases })}</td>
                <td class="number">
                  <Badge size="sm" tone={each.accuracy >= agent.min_accuracy ? 'success' : 'danger'}>{t('app.agents.percent', { value: each.accuracy })}</Badge>
                </td>
              </tr>
            {/each}
            {#snippet empty()}{t('app.agents.runs_empty')}{/snippet}
          </Table>
        {/if}
      </Stack>
    {/if}
  </Tabs>
</Stack>

{#if isRunOpen}
  <Dialog bind:isOpen={isRunOpen} title={t('app.agents.run_on_named', { name: agent.name })} dismissLabel={t('app.agents.cancel')}>
    <Stack gap="150">
      {#if target === 'HUB'}
        <Select label={t('app.agents.target_hub')} options={hubOptions} bind:value={chosenHub} />
      {:else}
        <p class="meta">{t('app.agents.target_entry')}</p>
        <EntryPicker chosen={chosenEntry} onchoose={(item) => (chosenEntry = item)} />
      {/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isRunOpen = false)}>{t('app.agents.cancel')}</Button>
      <Button tone="primary" disabledReason={runTarget ? undefined : t('app.agents.target_needed')} onclick={() => void run()}>{t('app.agents.run')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if isCaseOpen}
  <Dialog bind:isOpen={isCaseOpen} title={t('app.agents.add_case')} dismissLabel={t('app.agents.cancel')}>
    <Stack gap="150">
      <Input label={t('app.agents.case_title')} bind:value={caseTitle} maxlength={500} />
      <Textarea label={t('app.agents.case_notes')} rows={3} bind:value={caseNotes} />
      <Textarea label={t('app.agents.case_expected')} hint={t('app.agents.case_expected_hint')} rows={4} bind:value={caseExpected} />
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isCaseOpen = false)}>{t('app.agents.cancel')}</Button>
      <Button tone="primary" disabledReason={caseReason} onclick={() => void addCase()}>{t('app.agents.add')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row {
    display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .what { display: grid; gap: var(--sp-025); min-width: 0; }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; overflow-wrap: anywhere; }
  .section { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .number { text-align: end; font-variant-numeric: tabular-nums; }
</style>
