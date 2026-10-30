<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Changes* (configuration/ui.md §1, config-document.md §5): the changes proposed by agents
  // waiting for a person (D-16), the drafts not yet published, the configuration's versions with a
  // rollback and two of them compared, and the configuration file - the document every door reads,
  // never a second representation.
  //
  // **A rollback is a plan first**: the dialog shows what it changes and what it must have
  // acknowledged, and applies exactly that plan by its hash. **What the reader may never do is
  // not drawn** (P-05): without STRUCTURE the page lists and offers no control.

  import { untrack } from 'svelte';
  import { Banner, Button, Checkbox, CodeBlock, Dialog, PageHeader, Select, Spinner, Stack } from '@hubtask/design-system/components';
  import type { ConfigExport, ConfigPlan, ConfigProposal, ConfigProposalList, ConfigSandbox, ConfigVersion, ConfigVersionList, ResourceState } from '@hubtask/sync-engine';

  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { config, needsAcknowledge } from '../lib/data/config.svelte.ts';
  import { compareDocuments, type BlockDifference } from '../lib/data/configdiff.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { workflows } from '../lib/data/workflows.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import PlanReach from '../lib/settings/PlanReach.svelte';

  $effect(() => page.entitle(t('app.changes.title')));

  let versions = $state<ResourceState<ConfigVersionList>>();
  let waiting = $state<ResourceState<ConfigProposalList>>();
  let current = $state<ResourceState<ConfigExport>>();
  $effect(() => untrack(() => config.versions((next) => (versions = next))));
  $effect(() => untrack(() => config.waiting((next) => (waiting = next))));
  $effect(() => untrack(() => config.document((next) => (current = next))));

  const mayChange = $derived(holdsHere('STRUCTURE') === true);

  // *Drafts* (configuration/ui.md A4): the blocks edited and not yet published - today the
  // workflows, the one block kept as a draft - each opening where it is published.
  const workflowsOn = $derived(manifest.value?.modules_on?.includes('WORKFLOWS') === true);
  $effect(() => {
    if (!workflowsOn) return;
    return untrack(() => workflows.open());
  });
  const drafts = $derived((workflows.list?.data ?? []).filter((each) => each.draft !== null && each.draft !== undefined));
  const at = (iso: string) => formatDateTime(iso, messages.locale);
  const refusalOf = (state: ResourceState<unknown> | undefined) =>
    state?.status === 'failed' ? renderProblem(state.error, messages) : undefined;
  const counts = (each: { added: number; changed: number; removed: number }) =>
    t('app.changes.counts', { added: each.added, changed: each.changed, removed: each.removed });

  let busy = $state<string>();
  let acknowledged = $state<Record<string, boolean>>({});

  async function decide(id: string, decision: 'APPLY' | 'DECLINE') {
    busy = id;
    try {
      await config.decide(id, decision, acknowledged[id] === true);
      toast.show({ text: t(decision === 'APPLY' ? 'app.changes.applied' : 'app.changes.declined'), tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      busy = undefined;
    }
  }

  // An agent's proposal is shown with the same preview as any plan (config-document.md §3): planned
  // afresh against the configuration in force, as the reader, writing nothing.
  let previewing = $state<string>();
  let previewed = $state<ConfigPlan>();
  let isPreviewOpen = $state(false);

  async function preview(proposal: ConfigProposal) {
    previewing = proposal.id;
    try {
      previewed = await config.planProposal(proposal);
      isPreviewOpen = true;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      previewing = undefined;
    }
  }

  let rollingBack = $state<number>();
  let plan = $state<ConfigPlan>();
  let isRollbackOpen = $state(false);
  let acknowledgeRollback = $state(false);

  async function startRollback(version: number) {
    rollingBack = version;
    acknowledgeRollback = false;
    try {
      plan = await config.planRollback(version);
      isRollbackOpen = true;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }

  async function confirmRollback() {
    if (rollingBack === undefined || !plan?.plan_hash) return;
    try {
      await config.rollback(rollingBack, plan.plan_hash, acknowledgeRollback);
      toast.show({ text: t('app.changes.rolled_back', { version: rollingBack }), tone: 'success' });
      isRollbackOpen = false;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }

  // Two versions side by side: the older on the left, the newer on the right, each changed
  // block once. The choice starts at the last two versions.
  let left = $state<string>('');
  let right = $state<string>('');
  let compared = $state<BlockDifference[]>();
  let comparing = $state(false);
  let compareError = $state<string>();
  const versionOptions = $derived(
    versions?.status === 'ready'
      ? versions.data.data.map((each) => ({ value: String(each.version), label: t('app.changes.version', { version: each.version }) }))
      : [],
  );
  $effect(() => {
    if (versions?.status !== 'ready' || left !== '' || right !== '') return;
    const [newest, previous] = versions.data.data;
    if (newest && previous) {
      untrack(() => {
        left = String(previous.version);
        right = String(newest.version);
      });
    }
  });

  async function compare() {
    if (left === '' || right === '') return;
    comparing = true;
    compareError = undefined;
    try {
      const [a, b] = await Promise.all([config.version(Number(left)), config.version(Number(right))]);
      const documentOf = (state: ResourceState<ConfigVersion>) => (state.status === 'ready' ? state.data.document : undefined);
      const failed = a.status === 'failed' ? a : b.status === 'failed' ? b : undefined;
      if (failed) {
        compareError = renderProblem(failed.error, messages).message;
        compared = undefined;
        return;
      }
      compared = compareDocuments(documentOf(a) as never, documentOf(b) as never);
    } finally {
      comparing = false;
    }
  }
  const blockText = (block: Record<string, unknown> | undefined) => (block ? JSON.stringify(block, null, 2) : '');

  const file = $derived(current?.status === 'ready' ? JSON.stringify(current.data.document, null, 2) : '');

  // What an agent's apply does (D-16): read from the document in force, set by a person.
  type AgentApply = 'ALLOWED' | 'APPROVAL' | 'NEVER';
  const agentApply = $derived.by(() => {
    if (current?.status !== 'ready') return undefined;
    const settings = (current.data.document as { settings?: Record<string, { value?: unknown }> }).settings;
    const value = settings?.['config.agent_apply']?.value;
    return typeof value === 'string' ? value : undefined;
  });
  let agentApplyDraft = $state('');
  $effect(() => {
    agentApplyDraft = agentApply ?? '';
  });
  const agentApplyOptions = $derived((['APPROVAL', 'ALLOWED', 'NEVER'] as const).map((value) => ({ value, label: t('app.changes.agent_apply_option', { value }) })));
  let policyBusy = $state(false);
  async function saveAgentApply() {
    if (agentApplyDraft === '' || agentApplyDraft === agentApply) return;
    policyBusy = true;
    try {
      await config.setAgentApply(agentApplyDraft as AgentApply);
      toast.show({ text: t('app.changes.agent_apply_saved'), tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      policyBusy = false;
    }
  }

  // *Sandbox* (config-document.md §3): a copy where an agent changes freely and nothing runs;
  // promoting it applies what it changed since its copy, by the plan shown here.
  let sandbox = $state<ResourceState<ConfigSandbox>>();
  $effect(() => untrack(() => config.sandbox((next) => (sandbox = next))));
  const promotion = $derived(sandbox?.status === 'ready' && sandbox.data.exists ? sandbox.data.promotion : undefined);
  let acknowledgePromotion = $state(false);
  let sandboxBusy = $state(false);

  async function sandboxStep(step: () => Promise<unknown>, done: string) {
    sandboxBusy = true;
    try {
      await step();
      toast.show({ text: t(done), tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      sandboxBusy = false;
    }
  }
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.changes.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'changes', label: t('app.changes.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  <p class="quiet">{t('app.changes.intro')}</p>

  <section class="panel" data-proposals>
    <h2>{t('app.changes.proposed')}</h2>
    {#if refusalOf(waiting)}
      <Banner tone="danger" title={refusalOf(waiting)?.message ?? ''} />
    {:else if waiting?.status !== 'ready'}
      <Spinner label={t('app.changes.reading')} />
    {:else if waiting.data.data.length === 0}
      <p class="quiet">{t('app.changes.none_proposed')}</p>
    {:else}
      <ul class="rows">
        {#each waiting.data.data as proposal (proposal.id)}
          <li class="row" data-proposal={proposal.id}>
            <div class="identity">
              <span class="name">{t('app.changes.proposal', { at: at(proposal.created_at) })}</span>
              <span class="note">{counts(proposal)}</span>
              {#if proposal.template}<code class="note">{proposal.template}</code>{/if}
              {#if proposal.hides_data}<span class="flag">{t('app.changes.hides_data')}</span>{/if}
              {#if proposal.changes_permissions}<span class="flag">{t('app.changes.changes_permissions')}</span>{/if}
            </div>
            {#if mayChange}
              <Stack gap="100">
                {#if needsAcknowledge(proposal)}
                  <Checkbox
                    label={t('app.changes.acknowledge')}
                    checked={acknowledged[proposal.id] === true}
                    onchange={(event: Event) => (acknowledged = { ...acknowledged, [proposal.id]: (event.currentTarget as HTMLInputElement).checked })}
                  />
                {/if}
                <div class="actions">
                  <Button tone="primary" size="sm" isBusy={busy === proposal.id} data-apply={proposal.id}
                    disabledReason={needsAcknowledge(proposal) && !acknowledged[proposal.id] ? t('app.changes.acknowledge_first') : undefined}
                    onclick={() => decide(proposal.id, 'APPLY')}>{t('app.changes.apply')}</Button>
                  <Button tone="subtle" size="sm" data-decline={proposal.id} onclick={() => decide(proposal.id, 'DECLINE')}>{t('app.changes.decline')}</Button>
                  <Button tone="subtle" size="sm" isBusy={previewing === proposal.id} data-preview={proposal.id} onclick={() => preview(proposal)}>{t('app.changes.preview')}</Button>
                </div>
              </Stack>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  {#if agentApply !== undefined}
    <section class="panel" data-agent-apply>
      <h2>{t('app.changes.agent_apply')}</h2>
      <p class="quiet">{t('app.changes.agent_apply_explain')}</p>
      {#if mayChange}
        <Select label={t('app.changes.agent_apply')} bind:value={agentApplyDraft} options={agentApplyOptions} />
        <div class="actions">
          <Button tone="secondary" size="sm" isBusy={policyBusy} data-agent-apply-save onclick={() => void saveAgentApply()}>{t('app.changes.agent_apply_save')}</Button>
        </div>
      {:else}
        <p class="note">{t('app.changes.agent_apply_option', { value: agentApply })}</p>
      {/if}
    </section>
  {/if}

  <section class="panel" data-versions>
    <h2>{t('app.changes.history')}</h2>
    {#if refusalOf(versions)}
      <Banner tone="danger" title={refusalOf(versions)?.message ?? ''} />
    {:else if versions?.status !== 'ready'}
      <Spinner label={t('app.changes.reading')} />
    {:else if versions.data.data.length === 0}
      <p class="quiet">{t('app.changes.no_versions')}</p>
    {:else}
      <ul class="rows">
        {#each versions.data.data as version, index (version.version)}
          <li class="row" data-version={version.version}>
            <div class="identity">
              <span class="name">{t('app.changes.version', { version: version.version })}</span>
              <span class="note">{at(version.created_at)}</span>
              <span class="note">{t(version.actor_kind === 'AI_AGENT' ? 'app.changes.by_agent' : 'app.changes.by_person')}</span>
              {#if version.proposed_by}<span class="note">{t('app.changes.was_proposed')}</span>{/if}
              <span class="note">{counts(version)}</span>
              {#if version.template}<code class="note">{version.template}</code>{/if}
              {#if version.rolled_back_to}<span class="note">{t('app.changes.rollback_of', { version: version.rolled_back_to })}</span>{/if}
            </div>
            {#if mayChange && index > 0}
              <Button tone="subtle" size="sm" data-rollback={version.version} onclick={() => startRollback(version.version)}>{t('app.changes.roll_back')}</Button>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  {#if workflowsOn}
    <section class="panel" data-drafts>
      <h2>{t('app.changes.drafts')}</h2>
      {#if drafts.length === 0}
        <p class="quiet">{t('app.changes.no_drafts')}</p>
      {:else}
        <ul class="rows">
          {#each drafts as workflow (workflow.key)}
            <li class="row" data-draft={`workflows.${workflow.key}`}>
              <div class="identity">
                <code class="name">workflows.{workflow.key}</code>
                {#if workflow.name}<span class="note">{workflow.name}</span>{/if}
                <span class="note">{t('app.changes.draft_unpublished')}</span>
              </div>
              <a class="open" href="/administration/workflows">{t('app.changes.open_draft')}</a>
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  {/if}

  {#if versions?.status === 'ready' && versions.data.data.length > 1}
    <section class="panel" data-compare>
      <h2>{t('app.changes.compare')}</h2>
      <div class="pick">
        <Select label={t('app.changes.compare_from')} bind:value={left} options={versionOptions} />
        <Select label={t('app.changes.compare_to')} bind:value={right} options={versionOptions} />
        <Button tone="secondary" size="sm" isBusy={comparing} data-compare-run onclick={compare}>{t('app.changes.compare_run')}</Button>
      </div>
      {#if compareError}
        <Banner tone="danger" title={compareError} />
      {:else if compared && compared.length === 0}
        <p class="quiet">{t('app.changes.compare_same')}</p>
      {:else if compared}
        <ul class="rows">
          {#each compared as difference (difference.path)}
            <li class="difference" data-difference={difference.path}>
              <div class="identity">
                <code class="name">{difference.path}</code>
                <span class="note">{t(`app.changes.difference_${difference.kind.toLowerCase()}`)}</span>
              </div>
              <div class="sides">
                <pre class="side" aria-label={t('app.changes.compare_version', { version: left })}>{blockText(difference.before)}</pre>
                <pre class="side" aria-label={t('app.changes.compare_version', { version: right })}>{blockText(difference.after)}</pre>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  {/if}

  <section class="panel" data-sandbox>
    <h2>{t('app.changes.sandbox')}</h2>
    <p class="quiet">{t('app.changes.sandbox_explain')}</p>
    {#if refusalOf(sandbox)}
      <Banner tone="danger" title={refusalOf(sandbox)?.message ?? ''} />
    {:else if sandbox?.status !== 'ready'}
      <Spinner label={t('app.changes.reading')} />
    {:else if !sandbox.data.exists}
      <p class="quiet">{t('app.changes.sandbox_none')}</p>
      {#if mayChange}
        <div class="actions">
          <Button tone="secondary" size="sm" isBusy={sandboxBusy} data-sandbox-copy
            onclick={() => sandboxStep(() => config.resetSandbox(false), 'app.changes.sandbox_copied')}>{t('app.changes.sandbox_copy')}</Button>
        </div>
      {/if}
    {:else}
      <p class="note">{t('app.changes.sandbox_state', { writes: sandbox.data.writes ?? 0, at: at(sandbox.data.updated_at ?? '') })}</p>
      {#if promotion && promotion.refusals.length > 0}
        <Banner tone="danger" title={t('app.changes.sandbox_refused')} />
      {:else if promotion && promotion.changes.length === 0}
        <p class="quiet">{t('app.changes.sandbox_same')}</p>
      {:else if promotion}
        <p class="quiet">{counts(promotion.counts)}</p>
        <ul class="changes">
          {#each promotion.changes as change (change.path)}
            <li><code>{change.op}</code> {change.path}</li>
          {/each}
        </ul>
        <PlanReach reach={promotion.reach} />
        {#if mayChange && needsAcknowledge(promotion.flags)}
          <Checkbox label={t('app.changes.acknowledge')} checked={acknowledgePromotion}
            onchange={(event: Event) => (acknowledgePromotion = (event.currentTarget as HTMLInputElement).checked)} />
        {/if}
      {/if}
      {#if mayChange}
        <div class="actions">
          {#if promotion?.plan_hash && promotion.changes.length > 0}
            <Button tone="primary" size="sm" isBusy={sandboxBusy} data-sandbox-promote
              disabledReason={needsAcknowledge(promotion.flags) && !acknowledgePromotion ? t('app.changes.acknowledge_first') : undefined}
              onclick={() => sandboxStep(() => config.promote(promotion?.plan_hash ?? '', acknowledgePromotion), 'app.changes.sandbox_promoted')}>{t('app.changes.sandbox_promote')}</Button>
          {/if}
          <Button tone="subtle" size="sm" data-sandbox-reset
            onclick={() => sandboxStep(() => config.resetSandbox(false), 'app.changes.sandbox_copied')}>{t('app.changes.sandbox_reset')}</Button>
          <Button tone="subtle" size="sm" data-sandbox-discard
            onclick={() => sandboxStep(() => config.resetSandbox(true), 'app.changes.sandbox_discarded')}>{t('app.changes.sandbox_discard')}</Button>
        </div>
      {/if}
    {/if}
  </section>

  <section class="panel" data-file>
    <h2>{t('app.changes.file')}</h2>
    <p class="quiet">{t('app.changes.file_explain')}</p>
    {#if current?.status === 'ready'}
      <CodeBlock code={file} label={t('app.changes.file')} language="json" />
    {:else if refusalOf(current)}
      <Banner tone="danger" title={refusalOf(current)?.message ?? ''} />
    {:else}
      <Spinner label={t('app.changes.reading')} />
    {/if}
  </section>
</Stack>

{#if previewed}
  <Dialog bind:isOpen={isPreviewOpen} title={t('app.changes.preview_title')} dismissLabel={t('app.changes.cancel')}>
    <Stack gap="150">
      {#if previewed.refusals.length > 0}
        <Banner tone="danger" title={t('app.changes.preview_refused')}>
          <ul class="changes">
            {#each previewed.refusals as refused (refused.path + refused.code)}
              <li><code>{refused.path}</code> {refused.code}</li>
            {/each}
          </ul>
        </Banner>
      {:else}
        <p class="quiet">{counts(previewed.counts)}</p>
        <ul class="changes">
          {#each previewed.changes as change (change.path)}
            <li><code>{change.op}</code> {change.path}</li>
          {/each}
        </ul>
        <PlanReach reach={previewed.reach} />
      {/if}
    </Stack>
  </Dialog>
{/if}

{#if plan && rollingBack !== undefined}
  <Dialog bind:isOpen={isRollbackOpen} title={t('app.changes.rollback_title', { version: rollingBack })} dismissLabel={t('app.changes.cancel')}>
    <Stack gap="150">
      {#if plan.refusals.length > 0}
        <Banner tone="danger" title={t('app.changes.rollback_refused')} />
      {:else}
        <p class="quiet">{counts(plan.counts)}</p>
        <ul class="changes">
          {#each plan.changes as change (change.path)}
            <li><code>{change.op}</code> {change.path}</li>
          {/each}
        </ul>
        <PlanReach reach={plan.reach} />
        {#if needsAcknowledge(plan.flags)}
          <Checkbox label={t('app.changes.acknowledge')} checked={acknowledgeRollback}
            onchange={(event: Event) => (acknowledgeRollback = (event.currentTarget as HTMLInputElement).checked)} />
        {/if}
      {/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isRollbackOpen = false)}>{t('app.changes.cancel')}</Button>
      {#if plan?.plan_hash}
        <Button tone="primary" data-confirm-rollback
          disabledReason={plan && needsAcknowledge(plan.flags) && !acknowledgeRollback ? t('app.changes.acknowledge_first') : undefined}
          onclick={confirmRollback}>{t('app.changes.roll_back')}</Button>
      {/if}
    {/snippet}
  </Dialog>
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  h2 { margin: 0 0 var(--sp-150); font-size: var(--fs-200); }

  .panel {
    padding: var(--sp-250);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .rows, .changes { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }

  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--sp-100);
    padding-block: var(--sp-100);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }

  .row:first-child { border-block-start: none; }
  .identity { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); min-inline-size: 0; }
  .actions { display: inline-flex; gap: var(--sp-100); }
  .name { font-weight: var(--fw-medium); }
  .note { color: var(--text-subtle); font-size: var(--fs-075); }
  .flag { color: var(--text-danger); font-size: var(--fs-075); }

  .open { color: var(--text-brand); font-size: var(--fs-075); }
  .pick { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-150); margin-block-end: var(--sp-150); }
  .difference {
    display: grid;
    gap: var(--sp-100);
    padding-block: var(--sp-100);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }
  .difference:first-child { border-block-start: none; }
  .sides { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, var(--layout-compare-side)), 1fr)); gap: var(--sp-100); }
  .side {
    margin: 0;
    padding: var(--sp-100);
    min-block-size: var(--sp-300);
    overflow-x: auto;
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
  }
</style>
