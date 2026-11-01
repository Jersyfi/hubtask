<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installation's dead letters (UC-INS-12, alert A-07): the jobs that ran out of attempts and
  // wait for the operator, and the one act on them - running one again once its cause is fixed.
  //
  // **Codes and moments only.** The answer never carries what a job was given; the last failure is
  // a message code, rendered here where this build has a word for it and shown as the code where
  // it has none. A workspace is named by its address where the workspaces list has it.
  import { untrack } from 'svelte';

  import { Banner, Button, LoadMore, PageHeader, Spinner, Stack, Table } from '@hubtask/design-system/components';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { deadLetters } from '../lib/data/deadletters.svelte.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  $effect(() => {
    if (!instance.isElevated) return;
    const stops = untrack(() => [deadLetters.open(), instance.openWorkspaces()]);
    return () => stops.forEach((stop) => stop());
  });

  const reading = $derived(deadLetters.state);
  const letters = $derived(deadLetters.letters);
  const unreadable = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  const slugs = $derived(new Map(instance.workspaces.map((workspace) => [workspace.id, workspace.slug])));

  let working = $state<string | undefined>(undefined);
  let failure = $state<{ readonly message: string; readonly reference?: string } | undefined>(undefined);

  async function requeue(id: string): Promise<void> {
    working = id;
    failure = undefined;
    try {
      await deadLetters.requeue(id);
      announcer.say(t('app.instance.dead_letters_requeued'));
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('errors.internal', {}) };
    } finally {
      working = undefined;
    }
  }

  let arrived = $state<string | undefined>(undefined);
  let isLoadingMore = $state(false);
  let moreFailed = $state<string | undefined>(undefined);

  async function more(): Promise<void> {
    const before = deadLetters.letters.length;
    moreFailed = undefined;
    isLoadingMore = true;
    try {
      await deadLetters.more();
      arrived = t('app.instance.dead_letters_arrived', { count: deadLetters.letters.length - before });
    } catch (error) {
      moreFailed = renderProblem(error as never, messages).message;
    } finally {
      isLoadingMore = false;
    }
  }

  const columns = $derived([
    { id: 'finished', label: t('app.instance.column_when') },
    { id: 'kind', label: t('app.instance.dead_letters_column_kind') },
    { id: 'workspace', label: t('app.instance.column_workspace') },
    { id: 'failure', label: t('app.instance.dead_letters_column_failure') },
    { id: 'act', label: t('app.instance.dead_letters_column_act') },
  ]);

  /** The word for a failure's code, or the code where this build has none. */
  function wordFor(code: string): string {
    const rendered = messages.t(code, {});
    return rendered === code ? code : rendered;
  }

  $effect(() => page.entitle(t('app.instance.dead_letters')));
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.instance.dead_letters')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={installationTrail({ id: 'screen', label: t('app.instance.dead_letters') })}
  />

  <InstanceGate onleave={() => onnavigate('/')}>
    <Stack gap="200">
      <p class="prose">{t('app.instance.dead_letters_intro')}</p>
      {#if failure}
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
      {:else if letters.length === 0}
        <Banner tone="success">{t('app.instance.dead_letters_empty')}</Banner>
      {:else}
        <Table label={t('app.instance.dead_letters')} isLabelHidden columns={columns}>
          {#each letters as letter (letter.id)}
            <tr>
              <td class="quiet-cell">{letter.finished_at ? formatDateTime(letter.finished_at, messages.locale) : ''}</td>
              <td>
                <span class="strong mono">{letter.kind}</span>
                <span class="quiet-cell">{t('app.instance.dead_letters_attempts', { count: letter.attempts })}</span>
              </td>
              <td>
                {#if letter.tenant_id}
                  {#if slugs.get(letter.tenant_id)}
                    <span class="strong">{slugs.get(letter.tenant_id)}</span>
                  {:else}
                    <span class="mono">{letter.tenant_id.slice(0, 8)}…</span>
                  {/if}
                {:else}
                  <span class="quiet-cell">{t('app.instance.dead_letters_installation')}</span>
                {/if}
              </td>
              <td>
                {#if letter.error_code}
                  <span>{wordFor(letter.error_code)}</span>
                  <span class="quiet-cell mono">{letter.error_code}</span>
                {/if}
              </td>
              <td>
                <Button
                  isBusy={working === letter.id}
                  busyLabel={t('app.instance.working')}
                  disabledReason={working !== undefined && working !== letter.id ? t('app.instance.dead_letters_one_at_a_time') : undefined}
                  onclick={() => void requeue(letter.id)}
                >
                  {t('app.instance.dead_letters_requeue')}
                </Button>
              </td>
            </tr>
          {/each}
        </Table>
        <LoadMore
          label={t('app.instance.dead_letters_more')}
          busyLabel={t('app.instance.dead_letters_loading_more')}
          hasMore={Boolean(deadLetters.moreAfter)}
          isBusy={isLoadingMore}
          arrivedLabel={arrived}
          failedLabel={moreFailed}
          onLoadMore={() => void more()}
        />
      {/if}
    </Stack>
  </InstanceGate>
</Stack>

<style>
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .quiet-cell { display: block; color: var(--text-secondary); font-size: var(--fs-100); }
  .strong { display: block; font-weight: var(--fw-medium); }
  .mono { font-family: var(--font-mono); font-size: var(--fs-100); overflow-wrap: anywhere; }
</style>
