<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One backup target as its record page (page-system §7.2, §9.3): it has a list of its own, the
  // archives at it, so it is a page rather than a dialog. *Back up now* is the head's act, testing
  // the connection its ⋯, removing it the danger zone.
  //
  // **What is at the target is read from the target**: the listing reads the manifests where the
  // archives lie, then each archive's run - the half that says whether this installation has
  // opened it since. "We have backups" and "we have backups that open" stay two claims.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, DataTable, EmptyState, RecordPage, Spinner, Stack, type DataColumn, type RecordList } from '@hubtask/design-system/components';

  import { announcer } from '../lib/announce.svelte.ts';
  import JobProgress from '../lib/backup/JobProgress.svelte';
  import RemoveTargetDialog from '../lib/backup/RemoveTargetDialog.svelte';
  import { archiveState, lastVerified, sentence, testedWord, when } from '../lib/backup/words.svelte.ts';
  import { backup, type Archive } from '../lib/data/backup.svelte.ts';
  import { mayHere } from '../lib/data/capability.svelte.ts';
  import { jobs, type Watch } from '../lib/data/jobs.svelte.ts';
  import { isTerminal } from '../lib/data/jobs.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { formatBytes } from '../lib/i18n/bytes.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { Continued } from '../lib/page/continued.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import WorkspaceRefusal from '../lib/workspace/WorkspaceRefusal.svelte';

  interface Props {
    id: string;
    onnavigate: (path: string) => void;
  }

  const { id, onnavigate }: Props = $props();

  const LIST = '/administration/backup/targets';

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  let startedJob = $state<string | undefined>(undefined);
  let verifying = $state<string | undefined>(undefined);
  let isRemoving = $state(false);

  $effect(() => untrack(() => backup.open()));
  $effect(() => {
    const wanted = id;
    void untrack(() => backup.readArchives(wanted));
  });

  const target = $derived(backup.all.find((each) => each.id === id));
  const archives = $derived(backup.archivesAt(id));
  /** Newest first: the archive somebody is after is nearly always the last one written. */
  const listed = $derived<readonly Archive[]>(
    archives.status === 'ready' ? [...archives.data].sort((a, b) => b.created_at.localeCompare(a.created_at)) : [],
  );
  const probe = $derived(backup.probeOf(id));
  const watched = $derived<Watch | undefined>(startedJob ? jobs.of(startedJob) : undefined);
  const verified = $derived(lastVerified(listed, (run) => backup.runOf(run)));
  const shownArchives = new Continued(() => listed);

  const mayStructure = $derived(mayHere('STRUCTURE').status === 'permitted');
  const mayOwn = $derived(mayHere('DELETE_CONTAINER').status === 'permitted');

  // A finished job makes the archives on screen out of date: read them again, past the cache.
  $effect(() => {
    const watch = watched;
    if (!watch || watch.watching || !isTerminal(watch.job.status)) return;
    announcer.say(
      watch.job.status === 'SUCCEEDED'
        ? verifying ? t('app.backup.verify_done') : t('app.backup.run_done')
        : watch.job.status === 'CANCELLED' ? t('app.backup.run_cancelled') : t('app.backup.run_failed'),
    );
    void untrack(() => backup.readArchives(id, true));
  });

  async function attempt(work: () => Promise<unknown>, said?: string): Promise<void> {
    failure = undefined;
    isWorking = true;
    try {
      await work();
      if (said) announcer.say(said);
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }

  async function start(): Promise<void> {
    await attempt(async () => {
      const accepted = await backup.start({ target_id: id });
      startedJob = accepted.job_id;
      verifying = undefined;
    }, t('app.backup.started_announced'));
  }

  async function verify(archive: Archive): Promise<void> {
    await attempt(async () => {
      const accepted = await backup.verify(archive.archive_id);
      startedJob = accepted.job_id;
      verifying = archive.archive_id;
    }, t('app.backup.verify_started_announced'));
  }

  const title = $derived(target?.name ?? t('app.backup.target_word'));
  const status = $derived.by(() => {
    if (!target) return undefined;
    if (!target.last_test_at) return { label: t('app.backup.status_untested'), tone: 'neutral' as const };
    return target.last_test_ok
      ? { label: t('app.backup.status_answered'), tone: 'success' as const }
      : { label: t('app.backup.status_not_answering'), tone: 'danger' as const };
  });
  const lists = $derived<RecordList[]>(
    target ? [{ id: 'archives', title: t('app.backup.archives_title'), count: archives.status === 'ready' ? String(listed.length) : undefined }] : [],
  );
  const columns: readonly DataColumn[] = $derived([
    { id: 'when', label: t('app.backup.column_written'), priority: 1 },
    { id: 'mode', label: t('app.backup.column_mode'), priority: 2 },
    { id: 'size', label: t('app.backup.column_size'), priority: 3, align: 'end' },
    { id: 'checked', label: t('app.backup.column_checked'), priority: 3 },
    { id: 'state', label: t('app.backup.column_opens'), isStatus: true },
  ]);

  $effect(() => page.entitle(title));
</script>

<RecordPage
  record={{
    kind: t('app.backup.target_word'),
    title,
    back: { label: t('app.backup.targets_title'), href: LIST, onnavigate: () => onnavigate(LIST) },
    status,
    facts: target ? `${t(`app.backup.kind_${target.kind}`)} · ${target.encryption_mode === 'NONE' ? t('app.backup.encryption_none') : t('app.backup.encrypted')}` : undefined,
    edit: target && mayStructure ? { label: t('app.backup.start'), icon: 'cloud-upload', onclick: () => void start() } : undefined,
    menu: target && mayStructure
      ? { label: t('app.page.more_for', { name: title }), items: [{ id: 'test', label: t('app.backup.test') }], onselect: () => void attempt(() => backup.test(id), t('app.backup.tested_announced')) }
      : undefined,
  }}
  {lists}
  facts={target
    ? {
        title: t('app.backup.facts_title'),
        rows: [
          { label: t('app.backup.target_kind'), value: t(`app.backup.kind_${target.kind}`) },
          { label: t('app.backup.encryption'), value: target.encryption_mode === 'NONE' ? t('app.backup.encryption_none') : t('app.backup.encryption_aes') },
          { label: t('app.backup.column_tested'), value: testedWord(target) },
        ],
      }
    : undefined}
  danger={target && mayOwn
    ? { title: t('app.page.danger_zone'), acts: [{ id: 'remove', title: t('app.backup.remove_title'), cost: t('app.backup.remove_note'), actionLabel: t('app.backup.remove') }], onact: () => (isRemoving = true) }
    : undefined}
  refused={backup.targets.status === 'failed' || (backup.targets.status === 'ready' && !target) ? notHere : undefined}
>
  {#snippet list(item: RecordList)}
    {#if item.id === 'archives' && target}
      <Stack gap="150">
        <WorkspaceRefusal permission="STRUCTURE" />
        {#if failure}<FailureNote problem={failure} />{/if}
        {#each target.warnings ?? [] as warning (warning)}
          <!-- What the target says about itself, in the catalogue's words. -->
          <Banner tone="warning">{sentence(warning)}</Banner>
        {/each}
        {#if probe}
          <Banner tone={probe.ok ? 'success' : 'danger'}>
            {#if probe.ok}
              {t('app.backup.probe_ok', { ms: String(Math.round(probe.latency_ms)) })}
              {#if probe.free_bytes}· {t('app.backup.probe_free', { size: formatBytes(probe.free_bytes, messages.locale) })}{/if}
            {:else}
              {sentence(probe.error_code) ?? t('app.backup.probe_failed')}
              {#if !probe.writable}· {t('app.backup.probe_not_writable')}{/if}
            {/if}
          </Banner>
        {/if}
        {#if watched}
          <JobProgress
            watch={watched}
            isVerifying={verifying !== undefined}
            mayStop={mayStructure}
            onstop={() => void attempt(() => jobs.cancel(watched!.job.job_id), t('app.jobs.cancel_asked_announced'))}
          />
        {/if}
        {#if archives.status === 'failed'}
          <FailureNote problem={renderProblem(archives.error, messages)} />
        {:else}
          {#if archives.status === 'ready'}
            <p class="quiet">{verified ? t('app.backup.last_verified', { at: when(verified) ?? '' }) : t('app.backup.never_verified')}</p>
          {/if}
          <DataTable
            label={item.title}
            {columns}
            rows={shownArchives.shown}
            key={(archive: Archive) => archive.archive_id}
            rowLabel={(archive: Archive) => when(archive.created_at) ?? archive.path}
            isBusy={archives.status === 'loading'}
            menu={(archive: Archive) => (mayStructure && archive.complete !== false
              ? { label: t('app.page.more_for', { name: when(archive.created_at) ?? archive.path }), items: [{ id: 'verify', label: t('app.backup.verify') }] }
              : undefined)}
            onmenu={(archive: Archive) => void verify(archive)}
            actionsLabel={t('app.page.row_actions')}
            sheetDismissLabel={t('app.page.sheet_close')}
            sheetGrabLabel={t('app.page.sheet_grab')}
            shownLabel={listed.length === 0 ? undefined : shownArchives.shownLabel}
            more={shownArchives.more}
            perPage={shownArchives.perPage}
          >
            {#snippet cell(archive: Archive, column: DataColumn)}
              {@const run = backup.runOf(archive.archive_id)}
              {@const state = archiveState(archive, run)}
              {#if column.id === 'when'}
                <span class="name">{when(archive.created_at)}</span>
              {:else if column.id === 'mode'}
                <span class="quiet">{t(`app.backup.mode_${archive.mode}`)}</span>
              {:else if column.id === 'size'}
                <span class="quiet">{archive.size_bytes ? formatBytes(archive.size_bytes, messages.locale) : ''}</span>
              {:else if column.id === 'checked'}
                <span class="quiet">{run?.verified_at ? when(run.verified_at) : ''}</span>
              {:else}
                <Badge size="sm" tone={state.tone}>{state.label}</Badge>
              {/if}
            {/snippet}
            {#snippet verb(archive: Archive)}
              {#if mayStructure && archive.complete !== false}
                <Button size="sm" tone="subtle" isBusy={isWorking && verifying === archive.archive_id} busyLabel={t('app.backup.verifying')} onclick={() => void verify(archive)}>
                  {t('app.backup.verify')}
                </Button>
              {/if}
            {/snippet}
            {#snippet empty()}
              {#if archives.status === 'loading' || archives.status === 'idle'}
                <p class="quiet"><Spinner label={t('app.backup.reading_archives')} /> <span>{t('app.backup.reading_archives')}</span></p>
              {:else}
                <EmptyState kind="unused" title={t('app.backup.no_archives')} />
              {/if}
            {/snippet}
          </DataTable>
        {/if}
      </Stack>
    {/if}
  {/snippet}
</RecordPage>

<RemoveTargetDialog target={isRemoving ? target : undefined} onClose={() => (isRemoving = false)} onRemoved={() => onnavigate(LIST)} />

{#snippet notHere()}
  {#if backup.targets.status === 'failed'}
    <FailureNote problem={renderProblem(backup.targets.error, messages)} />
  {:else}
    <EmptyState kind="settled" title={t('app.backup.target_not_here')} />
  {/if}
{/snippet}

<style>
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .name { color: var(--text-primary); font-weight: var(--fw-medium); }
</style>
