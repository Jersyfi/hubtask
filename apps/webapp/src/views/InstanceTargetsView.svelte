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

  import { Badge, Banner, Button, Dialog, PageHeader, Spinner, Stack, Table } from '@hubtask/design-system/components';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import TargetForm from '../lib/backup/TargetForm.svelte';
  import type { Probe, Target } from '../lib/data/backup.svelte.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  let removing = $state<Target | undefined>(undefined);
  let working = $state<string | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let probes = $state<Record<string, Probe>>({});

  $effect(() => {
    const stop = untrack(() => instance.openTargets());
    return stop;
  });

  const reading = $derived(instance.targetsState);
  const targets = $derived(instance.targets);
  const unreadable = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);

  const columns = $derived([
    { id: 'target', label: t('app.instance.targets_column_target') },
    { id: 'where', label: t('app.instance.targets_column_where') },
    { id: 'test', label: t('app.instance.targets_column_test') },
    { id: 'actions', label: t('app.instance.column_actions'), isLabelHidden: true },
  ]);

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
          : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
      return false;
    } finally {
      working = undefined;
    }
  }

  $effect(() => page.entitle(t('app.instance.targets')));
</script>

<Stack gap="300">
  <PageHeader title={t('app.instance.targets')} isTitleInBar={viewport.isCompact} />

  <InstanceGate onleave={() => onnavigate('/')}>
    <Stack gap="200">
      <p class="prose">{t('app.instance.targets_intro')}</p>

      {#if failure && !removing}
        <Banner tone="danger" title={failure.message}>
          {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
        </Banner>
      {/if}

      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet">
          <Spinner label={t('app.instance.reading')} />
          <span>{t('app.instance.reading')}</span>
        </p>
      {:else if unreadable}
        <Banner tone="danger" title={unreadable.message}>
          {#if unreadable.reference}{t('app.error_reference', { request_id: unreadable.reference })}{/if}
        </Banner>
      {:else}
        {#if targets.length === 0}
          <Banner tone="info">{t('app.instance.targets_none')}</Banner>
        {:else}
          <Table label={t('app.instance.targets')} isLabelHidden columns={columns}>
            {#each targets as target (target.id)}
              <tr>
                <td>
                  <span class="name">{target.name}</span>
                  <span class="slug">{target.kind}</span>
                  {#if target.enabled === false}<Badge tone="neutral">{t('app.instance.targets_disabled')}</Badge>{/if}
                </td>
                <td><span class="slug mono">{where(target)}</span></td>
                <td><span class="slug">{lastTest(target)}</span></td>
                <td>
                  <div class="row">
                    <Button
                      tone="subtle"
                      isBusy={working === `test:${target.id}`}
                      busyLabel={t('app.instance.working')}
                      onclick={() =>
                        void run(`test:${target.id}`, async () => {
                          probes = { ...probes, [target.id]: await instance.testTarget(target.id) };
                        })}
                    >
                      {t('app.backup.test')}
                    </Button>
                    <Button tone="danger" onclick={() => ((removing = target), (failure = undefined))}>
                      {t('app.instance.targets_remove')}
                    </Button>
                  </div>
                </td>
              </tr>
            {/each}
          </Table>
        {/if}

        <TargetForm
          heading={t('app.instance.targets_add')}
          isWorking={working === 'add'}
          onadd={(draft) => run('add', () => instance.createTarget(draft), t('app.instance.targets_added_announced'))}
        />
      {/if}
    </Stack>
  </InstanceGate>
</Stack>

<Dialog
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
      <Banner tone="danger" title={failure.message}>
        {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
      </Banner>
    {/if}
    <Banner tone="warning">{t('app.instance.targets_remove_note', { name: removing?.name ?? '' })}</Banner>
  </Stack>
</Dialog>

<style>
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .name { display: block; font-weight: var(--fw-medium); }
  .slug { display: block; color: var(--text-secondary); font-size: var(--fs-100); }
  .mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
  .row { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
</style>
