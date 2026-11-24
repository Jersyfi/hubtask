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

  import { Button, DataTable, EmptyState, FilterBar, LogPage, Spinner, type DataColumn } from '@hubtask/design-system/components';
  import { TransportError, type DeadLetter } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { deadLetters } from '../lib/data/deadletters.svelte.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { formatDateTime, formatDay } from '../lib/i18n/datetime.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import LogRecordDialog from '../lib/instance/LogRecordDialog.svelte';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

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
          : { message: messages.t('app.something_went_wrong') };
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

  const columns = $derived<DataColumn[]>([
    { id: 'kind', label: t('app.instance.dead_letters_column_kind'), priority: 1 },
    { id: 'finished', label: t('app.instance.column_when'), priority: 3 },
    { id: 'workspace', label: t('app.instance.column_workspace'), priority: 2 },
    { id: 'failure', label: t('app.instance.dead_letters_column_failure'), priority: 2 },
  ]);

  const whereOf = (letter: DeadLetter): string =>
    letter.tenant_id ? (slugs.get(letter.tenant_id) ?? `${letter.tenant_id.slice(0, 8)}…`) : t('app.instance.dead_letters_installation');

  // The search narrows the failed jobs loaded, by job, workspace and failure.
  let search = $state('');
  const shown = $derived(letters.filter((letter) => matchesSearch(search, [letter.kind, whereOf(letter), letter.error_code ? wordFor(letter.error_code) : undefined])));

  let opened = $state<DeadLetter | undefined>(undefined);
  const openedFacts = $derived(
    opened
      ? [
          { label: t('app.instance.column_when'), value: opened.finished_at ? formatDateTime(opened.finished_at, messages.locale) : '' },
          { label: t('app.instance.column_workspace'), value: whereOf(opened) },
          { label: t('app.instance.dead_letters_tries'), value: t('app.instance.dead_letters_attempts', { count: opened.attempts }) },
          ...(opened.error_code ? [{ label: t('app.instance.dead_letters_column_failure'), value: wordFor(opened.error_code) }] : []),
        ]
      : [],
  );
  const openedSupport = $derived(
    opened
      ? [
          { label: t('app.instance.dead_letters_column_kind'), value: opened.kind },
          ...(opened.error_code ? [{ label: t('app.instance.dead_letters_support_code'), value: opened.error_code }] : []),
          ...(opened.tenant_id ? [{ label: t('app.instance.journal_support_workspace'), value: opened.tenant_id }] : []),
          { label: t('app.instance.dead_letters_support_id'), value: opened.id },
        ]
      : [],
  );

  /** The word for a failure's code, or the code where this build has none. */
  function wordFor(code: string): string {
    const rendered = messages.t(code, {});
    return rendered === code ? code : rendered;
  }

  $effect(() => page.entitle(t('app.instance.dead_letters')));
</script>

<InstanceGate onleave={() => onnavigate('/')}>
  {#snippet children(door)}
    <LogPage head={{ title: t('app.instance.dead_letters'), breadcrumb: installationTrail({ id: 'screen', label: t('app.instance.dead_letters') }) }} refused={door}>
      {#snippet intro()}<p class="quiet">{t('app.instance.dead_letters_intro')}</p>{/snippet}
      {#snippet filters()}
        <FilterBar
          searchLabel={t('app.instance.dead_letters_search')}
          clearSearchLabel={t('app.page.clear_search')}
          bind:search
          countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(letters.length) }) : undefined}
        />
      {/snippet}

      {#if failure}<FailureNote problem={failure} />{/if}
      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet"><Spinner label={t('app.instance.reading')} /><span>{t('app.instance.reading')}</span></p>
      {:else if unreadable}
        <FailureNote problem={unreadable} />
      {:else}
        <DataTable
          label={t('app.instance.dead_letters')}
          {columns}
          rows={shown}
          key={(letter: DeadLetter) => letter.id}
          onopen={(letter: DeadLetter) => (opened = letter)}
          rowLabel={(letter: DeadLetter) => letter.kind}
          group={(letter: DeadLetter) => (letter.finished_at ? formatDay(letter.finished_at, messages.locale) : '')}
          shownLabel={letters.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
          more={deadLetters.moreAfter || arrived || moreFailed
            ? {
                label: t('app.instance.dead_letters_more'),
                busyLabel: t('app.instance.dead_letters_loading_more'),
                hasMore: Boolean(deadLetters.moreAfter),
                isBusy: isLoadingMore,
                arrivedLabel: arrived,
                failedLabel: moreFailed,
                onLoadMore: () => void more(),
              }
            : undefined}
        >
          {#snippet cell(letter: DeadLetter, column: DataColumn)}
            {#if column.id === 'kind'}
              <span class="strong">{letter.kind}</span>
              <span class="quiet-cell">{t('app.instance.dead_letters_attempts', { count: letter.attempts })}</span>
            {:else if column.id === 'finished'}
              <span class="quiet">{letter.finished_at ? formatDateTime(letter.finished_at, messages.locale) : ''}</span>
            {:else if column.id === 'workspace'}
              <span>{whereOf(letter)}</span>
            {:else}
              <span>{letter.error_code ? wordFor(letter.error_code) : ''}</span>
            {/if}
          {/snippet}
          {#snippet verb(letter: DeadLetter)}{@render runAgain(letter)}{/snippet}
          {#snippet empty()}
            {#if letters.length === 0}
              <EmptyState kind="settled" title={t('app.instance.dead_letters_empty')} />
            {:else}
              <EmptyState kind="filtered" title={t('app.instance.dead_letters_none_found')}>
                {#snippet action()}<Button onclick={() => (search = '')}>{t('app.page.clear')}</Button>{/snippet}
              </EmptyState>
            {/if}
          {/snippet}
        </DataTable>
      {/if}
    </LogPage>
  {/snippet}
</InstanceGate>

<LogRecordDialog isOpen={opened !== undefined} title={opened?.kind ?? ''} facts={openedFacts} support={openedSupport} onClose={() => (opened = undefined)}>
  {#snippet extra()}{#if opened}{@render runAgain(opened)}{/if}{/snippet}
</LogRecordDialog>

{#snippet runAgain(letter: DeadLetter)}
  <Button
    tone="subtle"
    isBusy={working === letter.id}
    busyLabel={t('app.instance.working')}
    disabledReason={working !== undefined && working !== letter.id ? t('app.instance.dead_letters_one_at_a_time') : undefined}
    onclick={() => void requeue(letter.id)}
  >
    {t('app.instance.dead_letters_requeue')}
  </Button>
{/snippet}

<style>
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .quiet-cell { display: block; color: var(--text-secondary); font-size: var(--fs-100); }
  .strong { display: block; font-weight: var(--fw-medium); overflow-wrap: anywhere; }
</style>
