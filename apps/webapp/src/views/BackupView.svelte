<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where the archives go, when they are written, and whether they open.
  //
  // **One section per address** (page-system §7.3, §9.3, PS-9): *Schedule*, *Targets* and *Recent
  // runs*, each at `/administration/backup/<section>`, and *Restore*, the screen of its own the list
  // leads to. Every act here is its own write - a dialog's, a row's switch - so no section has a
  // save bar (PS-10). *Back up now* is the head's primary; a target opens its record page with its
  // archives; a new target, a new schedule and the whole workspace's download are Dialogs M.
  //
  // **The first screen that watches a job**, and the pattern the next three reuse. `POST /backups`
  // answers a `JobRef` rather than a result; the watching is `lib/data/jobs.svelte.ts`'s.
  //
  // **"We have backups" and "we have backups that open" are different claims**, and the screen
  // keeps them apart: a run says when it was last checked, and where none ever was it says so.
  //
  // **What is at the target is read from the target.** There is no listing of runs in the
  // contract, and that is deliberate rather than missing: the manifests where the archives lie
  // still answer after a total loss. *Recent runs* is therefore every target's archives, each with
  // its run's row - an archive's id is its run's id.
  //
  // **No credential is ever read back.** A target's credentials go in at creation and the contract
  // answers `config` without them; there is nothing here to show and nothing to ask for.

  import { untrack } from 'svelte';

  import { Badge, Button, DataTable, EmptyState, SettingsLayout, Spinner, Switch, type DataColumn, type SectionLink } from '@hubtask/design-system/components';

  import { adminTrail } from '../lib/administration/trail.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import BackUpNowDialog from '../lib/backup/BackUpNowDialog.svelte';
  import DownloadDialog from '../lib/backup/DownloadDialog.svelte';
  import JobProgress from '../lib/backup/JobProgress.svelte';
  import NewScheduleDialog from '../lib/backup/NewScheduleDialog.svelte';
  import NewTargetDialog from '../lib/backup/NewTargetDialog.svelte';
  import RemoveScheduleDialog from '../lib/backup/RemoveScheduleDialog.svelte';
  import RemoveTargetDialog from '../lib/backup/RemoveTargetDialog.svelte';
  import ScheduleDialog from '../lib/backup/ScheduleDialog.svelte';
  import { archiveState, lastVerified, nextRun, ruleSentence, testedWord, when } from '../lib/backup/words.svelte.ts';
  import { backup, type Archive, type Schedule, type Target } from '../lib/data/backup.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { mayHere } from '../lib/data/capability.svelte.ts';
  import { deviceZone } from '../lib/i18n/zone.ts';
  import { jobs, type Watch } from '../lib/data/jobs.svelte.ts';
  import { isTerminal } from '../lib/data/jobs.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { formatBytes } from '../lib/i18n/bytes.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { Continued } from '../lib/page/continued.svelte.ts';
  import TableSection from '../lib/page/TableSection.svelte';
  import { renderProblem } from '../lib/problem.ts';
  import WorkspaceRefusal from '../lib/workspace/WorkspaceRefusal.svelte';

  interface Props {
    /** The section the address names (`/administration/backup/:section`); none is the list. */
    section?: string;
    /** Opens another page: a target's, *Restore*. */
    onnavigate?: (path: string) => void;
    /** Replaces the address: a move between sections. */
    onreplace?: (path: string) => void;
  }

  const { section, onnavigate, onreplace }: Props = $props();

  const BASE = '/administration/backup';
  const RESTORE = '/administration/restore';
  /** Where the guided target writes inside the installation's backup volume. */
  const GUIDED_PATH = 'workspaces';

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  let isGuiding = $state(false);

  // What is on screen rather than what exists: the job a press started, and the dialogs open.
  let startedJob = $state<string | undefined>(undefined);
  /** The target whose archives the running job changes, read again when it ends. */
  let jobTarget = $state<string | undefined>(undefined);
  let verifying = $state<string | undefined>(undefined);
  let isAddingTarget = $state(false);
  let isAddingSchedule = $state(false);
  let isChoosingTarget = $state(false);
  let isDownloadOpen = $state(false);
  let openedSchedule = $state<Schedule | undefined>(undefined);
  let removing = $state<Target | undefined>(undefined);
  let removingSchedule = $state<Schedule | undefined>(undefined);

  $effect(() => untrack(() => backup.open()));

  const reading = $derived(backup.targets);
  const targets = $derived(backup.all);
  const refusal = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  const schedules = $derived(backup.schedules.status === 'ready' ? backup.schedules.data : ([] as readonly Schedule[]));
  const watched = $derived<Watch | undefined>(startedJob ? jobs.of(startedJob) : undefined);

  /**
   * What the reader may do here, as the server asks it: testing and running behind `STRUCTURE`,
   * adding, changing and removing a target or a schedule behind the owner's `DELETE_CONTAINER` -
   * which an administrator does not hold either.
   */
  const mayStructure = $derived(mayHere('STRUCTURE').status === 'permitted');
  const mayOwn = $derived(mayHere('DELETE_CONTAINER').status === 'permitted');
  /** Whether this workspace may add a target at all: in provider operation the operator's switch
   * (backup-restore.md §2). Where it is off, a form or a one-press start would only be refused. */
  const mayAddTarget = $derived(manifest.value?.features?.backup_targets === true);
  /** Every target on this machine's disk: one failure takes the archives with the data. */
  const onlyHere = $derived(targets.length > 0 && targets.every((target) => target.kind === 'LOCAL'));

  const SECTIONS = [
    { id: 'schedule', code: 'app.backup.schedules_title' },
    { id: 'targets', code: 'app.backup.targets_title' },
    { id: 'runs', code: 'app.backup.runs_title' },
    { id: 'restore', code: 'app.backup.restore_title' },
  ] as const;
  /** *Restore* is a screen of its own: a link in the list, never a section here. */
  const isKnown = (id: string | undefined): boolean => id !== undefined && id !== 'restore' && SECTIONS.some((each) => each.id === id);
  const shown = $derived(isKnown(section) ? (section as string) : 'schedule');
  const view = $derived<'list' | 'detail'>(isKnown(section) ? 'detail' : 'list');

  const nameOf = (targetId: string) => targets.find((target) => target.id === targetId)?.name ?? targetId;
  const addressOf = (target: Target) => `${BASE}/targets/${encodeURIComponent(target.id)}`;

  // A job that has just ended is what makes the rows on screen out of date; re-reading is the
  // answer rather than a second polling loop, since the watcher already knows when to stop.
  $effect(() => {
    const watch = watched;
    if (!watch || watch.watching || !isTerminal(watch.job.status)) return;
    // A job finishing is a status change nobody focused (4.1.3): said once, when it appears.
    announcer.say(
      watch.job.status === 'SUCCEEDED'
        ? verifying ? t('app.backup.verify_done') : t('app.backup.run_done')
        : watch.job.status === 'CANCELLED' ? t('app.backup.run_cancelled') : t('app.backup.run_failed'),
    );
    const target = jobTarget;
    // `refresh`: the server's listing of the target is from before the run that just ended.
    if (target && read.has(target)) void untrack(() => backup.readArchives(target, true));
  });

  async function attempt(work: () => Promise<unknown>, said?: string): Promise<void> {
    failure = undefined;
    isWorking = true;
    try {
      await work();
      // Said out loud on success (4.1.3): what changed is on the screen, and a reader who cannot
      // see it is told the same thing once, through the one live region.
      if (said) announcer.say(said);
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }

  async function start(targetId: string): Promise<void> {
    await attempt(async () => {
      const accepted = await backup.start({ target_id: targetId });
      startedJob = accepted.job_id;
      jobTarget = targetId;
      verifying = undefined;
    }, t('app.backup.started_announced'));
  }

  async function verify(row: RunRow): Promise<void> {
    await attempt(async () => {
      const accepted = await backup.verify(row.archive.archive_id);
      startedJob = accepted.job_id;
      jobTarget = row.target.id;
      verifying = row.archive.archive_id;
    }, t('app.backup.verify_started_announced'));
  }

  /** The head's primary: one target starts at once, several ask which. */
  function backUpNow(): void {
    if (targets.length === 1 && targets[0]) void start(targets[0].id);
    else isChoosingTarget = true;
  }

  /**
   * The empty screen's one act (user-experience.md §8.3): a target in the installation's backup
   * volume and a nightly schedule on it, as `backup-restore.md` §4's example - incremental each
   * night, full on Sunday with the trial restore, the server's default retention. The path is the
   * same for every workspace: an archive's name carries its workspace, and a target lists only its
   * own workspace's archives.
   */
  async function backUpNightly(): Promise<void> {
    isGuiding = true;
    await attempt(async () => {
      const target = await backup.createTarget({
        name: t('app.backup.guided_target_name'),
        kind: 'LOCAL',
        config: { path: GUIDED_PATH },
        encryption_mode: 'AES256_GCM',
      });
      await backup.createSchedule({
        target_id: target.id,
        scope: { kind: 'TENANT' },
        rrule: 'FREQ=DAILY;BYHOUR=3;BYMINUTE=0',
        timezone: deviceZone(),
        mode: 'INCREMENTAL',
        full_rrule: 'FREQ=WEEKLY;BYDAY=SU',
        trial_restore: true,
        notify_on: ['FAILURE', 'FIRST_SUCCESS_AFTER_FAILURE'],
      });
    }, t('app.backup.guided_done_announced'));
    isGuiding = false;
  }

  // Recent runs: each target's archives, read once the section is open - a read per archive's run
  // follows, which a generation plan bounds.
  const read = new Set<string>();
  $effect(() => {
    if (shown !== 'runs') return;
    const ids = targets.map((target) => target.id);
    untrack(() => {
      for (const id of ids) {
        if (read.has(id)) continue;
        read.add(id);
        void backup.readArchives(id);
      }
    });
  });

  interface RunRow {
    readonly target: Target;
    readonly archive: Archive;
  }

  const runRows = $derived<readonly RunRow[]>(
    targets
      .flatMap((target) => {
        const listing = backup.archivesAt(target.id);
        return listing.status === 'ready' ? listing.data.map((archive) => ({ target, archive })) : [];
      })
      .sort((a, b) => b.archive.created_at.localeCompare(a.archive.created_at)),
  );
  const runsReading = $derived(targets.some((target) => backup.archivesAt(target.id).status === 'loading'));
  const runsFailure = $derived.by(() => {
    for (const target of targets) {
      const listing = backup.archivesAt(target.id);
      if (listing.status === 'failed') return renderProblem(listing.error, messages);
    }
    return undefined;
  });
  const runs = new Continued(() => runRows);
  const verified = $derived(lastVerified(runRows.map((row) => row.archive), (id) => backup.runOf(id)));

  const scheduleColumns: readonly DataColumn[] = $derived([
    { id: 'rule', label: t('app.backup.column_when'), priority: 1 },
    { id: 'target', label: t('app.backup.schedule_target'), priority: 2 },
    { id: 'next', label: t('app.backup.column_next'), priority: 3 },
    { id: 'state', label: t('app.backup.enabled'), isStatus: true },
  ]);
  const targetColumns: readonly DataColumn[] = $derived([
    { id: 'name', label: t('app.backup.target_name'), priority: 1 },
    { id: 'kind', label: t('app.backup.target_kind'), priority: 2 },
    { id: 'tested', label: t('app.backup.column_tested'), priority: 3 },
    { id: 'encryption', label: t('app.backup.encryption'), isStatus: true },
  ]);
  const runColumns: readonly DataColumn[] = $derived([
    { id: 'when', label: t('app.backup.column_written'), priority: 1 },
    { id: 'target', label: t('app.backup.schedule_target'), priority: 2 },
    { id: 'mode', label: t('app.backup.column_mode'), priority: 3 },
    { id: 'size', label: t('app.backup.column_size'), priority: 3, align: 'end' },
    { id: 'state', label: t('app.backup.column_opens'), isStatus: true },
  ]);

  // A row's verb gives way on a phone, so each one is in its ⋯ too (PS-18).
  function scheduleMenu(schedule: Schedule) {
    if (!mayOwn) return undefined;
    return {
      label: t('app.page.more_for', { name: ruleSentence(schedule) }),
      items: [
        { id: 'switch', label: schedule.enabled === false ? t('app.backup.switch_on') : t('app.backup.switch_off') },
        { id: 'remove', label: t('app.backup.remove_schedule'), isDestructive: true, hasSeparatorBefore: true },
      ],
    };
  }

  function onScheduleMenu(schedule: Schedule, id: string): void {
    if (id === 'remove') removingSchedule = schedule;
    else void attempt(() => backup.updateSchedule(schedule.id, { enabled: schedule.enabled === false }), t('app.backup.schedule_saved_announced'));
  }

  function runMenu(row: RunRow) {
    if (!mayStructure || row.archive.complete === false) return undefined;
    return { label: t('app.page.more_for', { name: when(row.archive.created_at) ?? row.archive.path }), items: [{ id: 'verify', label: t('app.backup.verify') }] };
  }

  function targetMenu(target: Target) {
    return {
      label: t('app.page.more_for', { name: target.name }),
      items: [
        { id: 'open', label: t('app.page.open') },
        ...(mayStructure ? [{ id: 'test', label: t('app.backup.test') }, { id: 'start', label: t('app.backup.start') }] : []),
        ...(mayOwn ? [{ id: 'remove', label: t('app.backup.remove'), isDestructive: true, hasSeparatorBefore: true }] : []),
      ],
    };
  }

  function onTargetMenu(target: Target, id: string): void {
    if (id === 'open') onnavigate?.(addressOf(target));
    else if (id === 'test') void attempt(() => backup.test(target.id), t('app.backup.tested_announced'));
    else if (id === 'start') void start(target.id);
    else if (id === 'remove') removing = target;
  }

  /** A section's state in the list, from what is read: never a guess before the read answers. */
  function statusOf(id: string): Pick<SectionLink, 'status' | 'statusTone'> {
    if (reading.status !== 'ready') return {};
    if (id === 'schedule') {
      const on = schedules.filter((schedule) => schedule.enabled !== false).length;
      return on > 0 ? { status: t('app.backup.status_on', { count: String(on) }), statusTone: 'set' } : { status: t('app.backup.off') };
    }
    if (id === 'targets') return { status: String(targets.length) };
    return {};
  }

  const title = $derived(t('app.backup.title'));
  const links = $derived<SectionLink[]>(
    SECTIONS.map((each) => ({
      id: each.id,
      label: t(each.code),
      href: each.id === 'restore' ? RESTORE : `${BASE}/${each.id}`,
      ...statusOf(each.id),
    })),
  );

  function select(next: string): void {
    if (next === 'restore') onnavigate?.(RESTORE);
    else onreplace?.(`${BASE}/${next}`);
  }

  $effect(() => page.entitle(title));
</script>

{#snippet noTargets()}
  {#if !mayAddTarget}
    <EmptyState kind="unused" icon="archive" title={t('app.backup.operator_keeps_title')} description={t('app.backup.operator_keeps_description')} />
  {:else if mayOwn}
    <EmptyState kind="unused" icon="archive" title={t('app.backup.guided_title')} description={t('app.backup.guided_description')}>
      {#snippet action()}
        <Button tone="primary" isBusy={isGuiding} busyLabel={t('app.backup.guided_working')} onclick={() => void backUpNightly()}>
          {t('app.backup.guided_start')}
        </Button>
      {/snippet}
      {#snippet footnote()}{t('app.backup.guided_footnote')}{/snippet}
    </EmptyState>
  {:else}
    <EmptyState kind="unused" icon="archive" title={t('app.backup.no_targets')} />
  {/if}
{/snippet}

<SettingsLayout
  head={{
    title,
    subtitle: t('app.backup.intro'),
    breadcrumb: adminTrail({ id: 'backup', label: title }),
    primary: mayStructure
      ? {
          label: t('app.backup.start'),
          icon: 'cloud-upload',
          onclick: backUpNow,
          disabledReason: reading.status === 'ready' && targets.length === 0 ? t('app.backup.start_needs_target') : undefined,
        }
      : undefined,
    menu: mayOwn
      ? { label: t('app.page.more_for', { name: title }), items: [{ id: 'download', label: t('app.backup.download_menu') }], onselect: () => (isDownloadOpen = true) }
      : undefined,
  }}
  nav={{ label: t('app.settings_page.sections', { name: title }), sections: links, current: shown, onselect: select }}
  {view}
  back={{ label: title, onback: () => onreplace?.(BASE) }}
>
  <div class="content">
    <WorkspaceRefusal permission="STRUCTURE" />
    <WorkspaceRefusal permission="DELETE_CONTAINER" />
    {#if failure}<FailureNote problem={failure} />{/if}
    {#if watched}
      <JobProgress
        watch={watched}
        isVerifying={verifying !== undefined}
        mayStop={mayStructure}
        onstop={() => void attempt(() => jobs.cancel(watched!.job.job_id), t('app.jobs.cancel_asked_announced'))}
      />
    {/if}

    {#if reading.status === 'loading' || reading.status === 'idle'}
      <p class="waiting"><Spinner label={t('app.backup.reading')} /> <span>{t('app.backup.reading')}</span></p>
    {:else if refusal}
      <FailureNote problem={refusal} />
    {:else if shown === 'schedule'}
      <TableSection title={t('app.backup.schedules_title')} note={String(schedules.length)} description={t('app.backup.schedules_intro')}>
        {#snippet acts()}
          {#if mayOwn && targets.length > 0}
            <Button icon="plus" onclick={() => (isAddingSchedule = true)}>{t('app.backup.add_schedule')}</Button>
          {/if}
        {/snippet}
        {#if targets.length === 0}
          {@render noTargets()}
        {:else}
          <DataTable
            label={t('app.backup.schedules_title')}
            columns={scheduleColumns}
            rows={schedules}
            key={(schedule: Schedule) => schedule.id}
            onopen={(schedule: Schedule) => (openedSchedule = schedule)}
            rowLabel={ruleSentence}
            menu={scheduleMenu}
            onmenu={onScheduleMenu}
            actionsLabel={t('app.page.row_actions')}
            sheetDismissLabel={t('app.page.sheet_close')}
            sheetGrabLabel={t('app.page.sheet_grab')}
            shownLabel={schedules.length === 0 ? undefined : t('app.page.showing', { count: String(schedules.length) })}
          >
            {#snippet cell(schedule: Schedule, column: DataColumn)}
              {#if column.id === 'rule'}
                <span class="name">{ruleSentence(schedule)}</span>
              {:else if column.id === 'target'}
                <span class="quiet">{nameOf(schedule.target_id)}</span>
              {:else if column.id === 'next'}
                <span class="quiet">{nextRun(schedule)}</span>
              {:else}
                <Badge size="sm" tone={schedule.enabled === false ? 'neutral' : 'success'}>
                  {schedule.enabled === false ? t('app.backup.off') : t('app.backup.on')}
                </Badge>
              {/if}
            {/snippet}
            {#snippet verb(schedule: Schedule)}
              <!-- In place (page-system §5.1): `enabled` is one field with two values, written on
                   the switch. Without the permission the switch is absent (P-05) and the state
                   column reads it; the page's one notice says why. -->
              {#if mayOwn}
                <Switch
                  label={t('app.backup.enabled_for', { rule: ruleSentence(schedule) })}
                  isLabelHidden
                  checked={schedule.enabled !== false}
                  onchange={(event) =>
                    void attempt(
                      () => backup.updateSchedule(schedule.id, { enabled: (event.currentTarget as HTMLInputElement).checked }),
                      t('app.backup.schedule_saved_announced'),
                    )}
                />
              {/if}
            {/snippet}
            {#snippet empty()}
              <EmptyState kind="unused" title={t('app.backup.no_schedules')}>
                {#snippet action()}
                  {#if mayOwn}<Button onclick={() => (isAddingSchedule = true)}>{t('app.backup.add_schedule')}</Button>{/if}
                {/snippet}
              </EmptyState>
            {/snippet}
          </DataTable>
        {/if}
      </TableSection>
    {:else if shown === 'targets'}
      <TableSection title={t('app.backup.targets_title')} note={String(targets.length)} description={t('app.backup.targets_intro')}>
        {#snippet acts()}
          {#if mayOwn && mayAddTarget}
            <Button icon="plus" onclick={() => (isAddingTarget = true)}>{t('app.backup.add_target')}</Button>
          {/if}
        {/snippet}
        <DataTable
          label={t('app.backup.targets_title')}
          columns={targetColumns}
          rows={targets}
          key={(target: Target) => target.id}
          href={addressOf}
          rowLabel={(target: Target) => target.name}
          menu={targetMenu}
          onmenu={onTargetMenu}
          actionsLabel={t('app.page.row_actions')}
          sheetDismissLabel={t('app.page.sheet_close')}
          sheetGrabLabel={t('app.page.sheet_grab')}
          shownLabel={targets.length === 0 ? undefined : t('app.page.showing', { count: String(targets.length) })}
        >
          {#snippet cell(target: Target, column: DataColumn)}
            {#if column.id === 'name'}
              <span class="name">{target.name}</span>
            {:else if column.id === 'kind'}
              <span class="quiet">{t(`app.backup.kind_${target.kind}`)}</span>
            {:else if column.id === 'tested'}
              <span class="quiet">{testedWord(target)}</span>
            {:else if target.encryption_mode === 'NONE'}
              <Badge size="sm" tone="warning">{t('app.backup.encryption_none')}</Badge>
            {:else}
              <Badge size="sm" tone="success">{t('app.backup.encrypted')}</Badge>
            {/if}
          {/snippet}
          {#snippet empty()}{@render noTargets()}{/snippet}
        </DataTable>
        {#if onlyHere && mayOwn && mayAddTarget}
          <!-- A suggestion, not a warning: nothing is wrong yet, so the mark is the accent. -->
          <aside class="suggestion" aria-labelledby="backup-offsite">
            <h3 class="minor" id="backup-offsite">{t('app.backup.offsite_title')}</h3>
            <p class="quiet">{t('app.backup.offsite_note')}</p>
          </aside>
        {/if}
      </TableSection>
    {:else if shown === 'runs'}
      <TableSection
        title={t('app.backup.runs_title')}
        note={runRows.length > 0 ? String(runRows.length) : undefined}
        description={runRows.length === 0 ? t('app.backup.runs_intro') : verified ? t('app.backup.last_verified', { at: when(verified) ?? '' }) : t('app.backup.never_verified')}
      >
        {#if targets.length === 0}
          {@render noTargets()}
        {:else if runsFailure}
          <FailureNote problem={runsFailure} />
        {:else}
          <DataTable
            label={t('app.backup.runs_title')}
            columns={runColumns}
            rows={runs.shown}
            key={(row: RunRow) => `${row.target.id}/${row.archive.archive_id}`}
            rowLabel={(row: RunRow) => when(row.archive.created_at) ?? row.archive.path}
            isBusy={runsReading}
            menu={runMenu}
            onmenu={(row: RunRow) => void verify(row)}
            actionsLabel={t('app.page.row_actions')}
            sheetDismissLabel={t('app.page.sheet_close')}
            sheetGrabLabel={t('app.page.sheet_grab')}
            shownLabel={runRows.length === 0 ? undefined : runs.shownLabel}
            more={runs.more}
            perPage={runs.perPage}
          >
            {#snippet cell(row: RunRow, column: DataColumn)}
              {@const state = archiveState(row.archive, backup.runOf(row.archive.archive_id))}
              {#if column.id === 'when'}
                <span class="name">{when(row.archive.created_at)}</span>
              {:else if column.id === 'target'}
                <span class="quiet">{row.target.name}</span>
              {:else if column.id === 'mode'}
                <span class="quiet">{t(`app.backup.mode_${row.archive.mode}`)}</span>
              {:else if column.id === 'size'}
                <span class="quiet">{row.archive.size_bytes ? formatBytes(row.archive.size_bytes, messages.locale) : ''}</span>
              {:else}
                <Badge size="sm" tone={state.tone}>{state.label}</Badge>
              {/if}
            {/snippet}
            {#snippet verb(row: RunRow)}
              {#if mayStructure && row.archive.complete !== false}
                <Button size="sm" tone="subtle" isBusy={isWorking && verifying === row.archive.archive_id} busyLabel={t('app.backup.verifying')} onclick={() => void verify(row)}>
                  {t('app.backup.verify')}
                </Button>
              {/if}
            {/snippet}
            {#snippet empty()}
              {#if runsReading}
                <p class="waiting"><Spinner label={t('app.backup.reading_archives')} /> <span>{t('app.backup.reading_archives')}</span></p>
              {:else}
                <EmptyState kind="unused" title={t('app.backup.no_runs')} />
              {/if}
            {/snippet}
          </DataTable>
        {/if}
      </TableSection>
    {/if}
  </div>
</SettingsLayout>

<NewTargetDialog bind:isOpen={isAddingTarget} />
<NewScheduleDialog bind:isOpen={isAddingSchedule} {targets} />
<BackUpNowDialog bind:isOpen={isChoosingTarget} {targets} {isWorking} onstart={(id) => void start(id)} />
<DownloadDialog bind:isOpen={isDownloadOpen} />
<ScheduleDialog schedule={openedSchedule} targetName={openedSchedule ? nameOf(openedSchedule.target_id) : ''} onClose={() => (openedSchedule = undefined)} />
<RemoveTargetDialog target={removing} onClose={() => (removing = undefined)} />
<RemoveScheduleDialog schedule={removingSchedule} onClose={() => (removingSchedule = undefined)} />

<style>
  .content { display: flex; flex-direction: column; gap: var(--sp-250); min-width: 0; }
  .quiet { margin: 0; color: var(--text-secondary); }
  .name { color: var(--text-primary); font-weight: var(--fw-medium); }
  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .suggestion {
    display: flex;
    flex-direction: column;
    gap: var(--sp-050);
    padding: var(--sp-200) var(--sp-250);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-inline-start: var(--bw-thick) solid var(--accent-primary);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
  .minor { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
</style>
