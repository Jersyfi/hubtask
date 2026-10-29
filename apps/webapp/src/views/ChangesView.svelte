<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Changes* (configuration/ui.md §1, config-document.md §5): the changes proposed by agents
  // waiting for a person (D-16), the configuration's versions with a rollback, and the
  // configuration file - the document every door reads, never a second representation.
  //
  // **A rollback is a plan first**: the dialog shows what it changes and what it must have
  // acknowledged, and applies exactly that plan by its hash. **What the reader may never do is
  // not drawn** (P-05): without STRUCTURE the page lists and offers no control.

  import { untrack } from 'svelte';
  import { Banner, Button, Checkbox, CodeBlock, Dialog, PageHeader, Spinner, Stack } from '@hubtask/design-system/components';
  import type { ConfigExport, ConfigPlan, ConfigProposalList, ConfigVersionList, ResourceState } from '@hubtask/sync-engine';

  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { config, needsAcknowledge } from '../lib/data/config.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  $effect(() => page.entitle(t('app.changes.title')));

  let versions = $state<ResourceState<ConfigVersionList>>();
  let waiting = $state<ResourceState<ConfigProposalList>>();
  let current = $state<ResourceState<ConfigExport>>();
  $effect(() => untrack(() => config.versions((next) => (versions = next))));
  $effect(() => untrack(() => config.waiting((next) => (waiting = next))));
  $effect(() => untrack(() => config.document((next) => (current = next))));

  const mayChange = $derived(holdsHere('STRUCTURE') === true);
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

  const file = $derived(current?.status === 'ready' ? JSON.stringify(current.data.document, null, 2) : '');
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
                </div>
              </Stack>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </section>

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
</style>
