<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installation's own backup targets (multi-tenancy.md §5, UC-BAK-07 check 1): where a
  // workspace that has no target of its own is exported to - in provider operation with tenant
  // targets off, that is every workspace.
  //
  // **No workspace sees one.** They belong to the installation; a workspace's own backup screen
  // lists only its own targets, and the export dialog shows these by name.
  //
  // **Configuring and removing need a fresh proof**, beyond the raised hour: a storage credential
  // every workspace's export may be written with is a privileged write. The credential goes in once
  // and never comes back.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, DataTable, Dialog, EmptyState, FilterBar, ListPage, Spinner, Stack, type DataColumn } from '@hubtask/design-system/components';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import NewTargetDialog from '../lib/backup/NewTargetDialog.svelte';
  import type { Probe, Target } from '../lib/data/backup.svelte.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  let removing = $state<Target | undefined>(undefined);
  let working = $state<string | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let probes = $state<Record<string, Probe>>({});

  $effect(() => {
    if (!instance.isElevated) return;
    const stop = untrack(() => instance.openTargets());
    return stop;
  });

  const reading = $derived(instance.targetsState);
  const targets = $derived(instance.targets);
  const unreadable = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);

  const columns = $derived<DataColumn[]>([
    { id: 'target', label: t('app.instance.targets_column_target'), priority: 1 },
    { id: 'where', label: t('app.instance.targets_column_where'), priority: 3 },
    { id: 'test', label: t('app.instance.targets_column_test'), priority: 2 },
  ]);

  let search = $state('');
  const shown = $derived(targets.filter((target) => matchesSearch(search, [target.name, target.kind])));
  let isAdding = $state(false);

  /** Where a target points, as its own settings say - the operator's to read, and no workspace's. */
  const where = (target: Target): string =>
    Object.entries(target.config ?? {})
      .map(([name, value]) => `${name}: ${String(value)}`)
      .join(' · ');

  const lastTest = (target: Target): string => {
    const probe = probes[target.id];
    if (probe) {
      return probe.ok ? t('app.backup.probe_ok', { ms: String(probe.latency_ms) }) : t('app.backup.probe_failed');
    }
    if (!target.last_test_at) return t('app.instance.targets_never_tested');
    const at = formatDateTime(target.last_test_at, messages.locale);
    return target.last_test_ok ? t('app.instance.targets_tested_ok', { at }) : t('app.instance.targets_tested_failed', { at });
  };

  async function run(id: string, call: () => Promise<unknown>, said?: string): Promise<boolean> {
    working = id;
    failure = undefined;
    try {
      await call();
      if (said) announcer.say(said);
      return true;
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('app.something_went_wrong'), fields: new Map(), isServerFault: true };
      return false;
    } finally {
      working = undefined;
    }
  }

  $effect(() => page.entitle(t('app.instance.targets')));
</script>

<InstanceGate onleave={() => onnavigate('/')}>
  {#snippet children(door)}
    <ListPage
      head={{
        title: t('app.instance.targets'),
        breadcrumb: installationTrail({ id: 'screen', label: t('app.instance.targets') }),
        primary: { label: t('app.instance.targets_add'), icon: 'plus', onclick: () => (isAdding = true) },
      }}
      refused={door ?? (unreadable ? failed : undefined)}
    >
      {#snippet intro()}<p class="quiet">{t('app.instance.targets_intro')}</p>{/snippet}
      {#snippet filters()}
        <FilterBar
          searchLabel={t('app.instance.targets_search')}
          clearSearchLabel={t('app.page.clear_search')}
          bind:search
          countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(targets.length) }) : undefined}
        />
      {/snippet}

      {#if failure && !removing}<FailureNote problem={failure} />{/if}
      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet"><Spinner label={t('app.instance.reading')} /><span>{t('app.instance.reading')}</span></p>
      {:else}
        <DataTable
          label={t('app.instance.targets')}
          {columns}
          rows={shown}
          key={(target: Target) => target.id}
          rowLabel={(target: Target) => target.name}
          menu={(target: Target) => ({
            label: t('app.page.more_for', { name: target.name }),
            items: [
              { id: 'test', label: t('app.backup.test') },
              { id: 'remove', label: `${t('app.instance.targets_remove')}…`, isDestructive: true, hasSeparatorBefore: true },
            ],
          })}
          onmenu={(target: Target, id: string) => {
            if (id === 'test') {
              void run(`test:${target.id}`, async () => {
                probes = { ...probes, [target.id]: await instance.testTarget(target.id) };
              });
            } else {
              removing = target;
              failure = undefined;
            }
          }}
          actionsLabel={t('app.page.row_actions')}
          sheetDismissLabel={t('app.page.sheet_close')}
          sheetGrabLabel={t('app.page.sheet_grab')}
          shownLabel={targets.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
        >
          {#snippet cell(target: Target, column: DataColumn)}
            {#if column.id === 'target'}
              <span class="name">{target.name}</span>
              <span class="slug">{t(`app.backup.kind_${target.kind}`)}</span>
              {#if target.enabled === false}<Badge tone="neutral">{t('app.instance.targets_disabled')}</Badge>{/if}
            {:else if column.id === 'where'}
              <span class="slug mono">{where(target)}</span>
            {:else}
              <span class="slug">{working === `test:${target.id}` ? t('app.instance.working') : lastTest(target)}</span>
            {/if}
          {/snippet}
          {#snippet empty()}
            {#if targets.length === 0}
              <EmptyState kind="unused" title={t('app.instance.targets_none')} />
            {:else}
              <EmptyState kind="filtered" title={t('app.instance.targets_none_found')}>
                {#snippet action()}<Button onclick={() => (search = '')}>{t('app.page.clear')}</Button>{/snippet}
              </EmptyState>
            {/if}
          {/snippet}
        </DataTable>
      {/if}
    </ListPage>
  {/snippet}
</InstanceGate>

<NewTargetDialog bind:isOpen={isAdding} create={(draft) => instance.createTarget(draft)} addedLabel={t('app.instance.targets_added_announced')} />

{#snippet failed()}
  {#if unreadable}<FailureNote problem={unreadable} />{/if}
{/snippet}

<Dialog
  size="s"
  title={t('app.instance.targets_remove_title')}
  isOpen={removing !== undefined}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (removing = undefined)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (removing = undefined)}>{t('app.instance.keep')}</Button>
    <Button
      tone="danger"
      isBusy={working === 'remove'}
      busyLabel={t('app.instance.working')}
      onclick={async () => {
        const removed = await run('remove', () => instance.removeTarget(removing?.id ?? ''), t('app.instance.targets_removed_announced'));
        if (removed) removing = undefined;
      }}
    >
      {t('app.instance.targets_remove')}
    </Button>
  {/snippet}
  <Stack gap="150">
    {#if failure}
      <FailureNote problem={failure} />
    {/if}
    <Banner tone="warning">{t('app.instance.targets_remove_note', { name: removing?.name ?? '' })}</Banner>
  </Stack>
</Dialog>

<style>
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .name { display: block; font-weight: var(--fw-medium); }
  .slug { display: block; color: var(--text-secondary); font-size: var(--fs-100); }
  .mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
</style>
