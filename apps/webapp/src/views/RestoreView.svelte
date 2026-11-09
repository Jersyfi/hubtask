<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The most dangerous screen in the product, and the one built to make the safe path the easy one.
  //
  // **A create page in three steps** (page-system §7.7, §9.3): *Archive*, *Plan* and *Confirm* as a
  // numbered segmented control under the head, each a region of setting rows at the content's full
  // width. *See what it would do* is the head's primary on every step: pressed with something
  // missing, it opens the step that misses it and marks the field (UC-WRK-32 check 3).
  //
  // **A restore is always rehearsed first.** `dry_run` is true in the contract and this screen
  // never flips it silently: the dry run goes first, its report is shown - what would be created,
  // overwritten, skipped - and only then is the *same request* offered without it. The second
  // request is composed from the first (`asked`), so what was reviewed is what runs.
  //
  // **`REPLACE_TENANT` ends credentials, and the screen says so before the confirmation.** The
  // mode replaces the workspace's rows, and the accounts, sessions and tokens that could sign in
  // to it are among them.
  //
  // **Three doors, in this order.** The exact workspace name typed; a step-up token, carried in
  // the *body* because this is the one operation where the contract puts the proof there; and
  // `create_safety_backup`, on by default, whose switching off is its own confirmation naming what
  // it gives up.
  //
  // **`INSTANCE` and `NEW_TENANT` are not offered**, and the reason is stated rather than left as
  // an absence: they cross or create a tenant, which is the installation operator's business.

  import { tick, untrack } from 'svelte';

  import {
    Badge,
    Banner,
    Button,
    Checkbox,
    CreatePage,
    FormLayout,
    FormSection,
    Input,
    ProgressBar,
    Select,
    SettingRow,
    Spinner,
    Stack,
    Switch,
    ViewSwitcher,
  } from '@hubtask/design-system/components';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  import { adminTrail } from '../lib/administration/trail.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { backup, type Archive } from '../lib/data/backup.svelte.ts';
  import { holds as roleHolds } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { jobs, type Watch } from '../lib/data/jobs.svelte.ts';
  import { isTerminal, mayCancel } from '../lib/data/jobs.ts';
  import { restores, type Asked, type Mode, type Run } from '../lib/data/restore.svelte.ts';
  import RestoreReport from '../lib/restore/RestoreReport.svelte';
  import { isDestructive, mayRunForReal, offeredModes } from '../lib/data/restore.ts';
  import { policies } from '../lib/data/policies.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { stepUp } from '../lib/data/stepup.svelte.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { formatBytes } from '../lib/i18n/bytes.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { check, focusFirst, isComplete, type Missing, type Requirement } from '../lib/forms/missing.ts';
  import { mayHere } from '../lib/data/capability.svelte.ts';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';
  import { page } from '../lib/frame/page.svelte.ts';

  /** What a collision may become. `SKIP` is the contract's default and the safe one. */
  const CONFLICT_RULES = ['SKIP', 'OVERWRITE', 'DUPLICATE'];

  type Step = 'archive' | 'plan' | 'confirm';
  const STEPS: readonly Step[] = ['archive', 'plan', 'confirm'];
  /** The step each requirement is asked on, so a press of the primary opens the right one. */
  const STEP_OF: Readonly<Record<string, Step>> = {
    target: 'archive',
    archive: 'archive',
    selection: 'plan',
    cost: 'confirm',
    name: 'confirm',
    safety: 'confirm',
  };

  let step = $state<Step>('archive');
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  let targetId = $state('');
  let archiveId = $state('');
  let mode = $state<string>('INSPECT');
  let conflictRule = $state('SKIP');
  let safetyBackup = $state(true);
  let safetyOffAcknowledged = $state(false);
  let typedName = $state('');
  /** Only for an archive downloaded with a passphrase; the server refuses one any other has no use for. */
  let passphrase = $state('');
  let costAcknowledged = $state(false);
  /** Which containers a selective restore names. Composed, never typed. */
  let chosen = $state<readonly string[]>([]);

  /** What the dry run was asked for. The second request is this one with `dry_run: false`. */
  let asked = $state<Asked | undefined>(undefined);
  /**
   * The form as it stood when the rehearsal was asked for. A report describes one request, so it
   * is shown only while the form still matches it - else the button beneath it would run the old
   * one, which is right and reads as wrong.
   */
  let rehearsedFrom = $state('');
  let rehearsalId = $state('');
  let realId = $state('');
  let watchedJob = $state('');

  $effect(() => untrack(() => backup.open()));

  // Replacing the workspace is the owner's (DELETE_CONTAINER, backup-restore.md §8.2); the other
  // modes are anybody's who reaches this screen. A prediction; the server decides (P-05).
  $effect(() => untrack(() => people.open({})));
  const workspaceRole = $derived(
    people.along({}).find((membership) => membership.account_id === actor.account?.id)?.role as string | undefined,
  );
  const mayReplace = $derived(roleHolds(workspaceRole, 'DELETE_CONTAINER').status === 'permitted');
  const modes = $derived(offeredModes(mayReplace));
  // A legal hold in force stops the real replace (backup-restore.md §8.2); its rehearsal stays.
  $effect(() => untrack(() => policies.openHolds(false)));
  const holdInForce = $derived(Array.isArray(policies.placed) && policies.placed.length > 0);
  // A mode no longer offered - the role was read after the choice - falls back to the safe one.
  $effect(() => {
    if (!(modes as readonly string[]).includes(mode)) mode = 'INSPECT';
  });
  $effect(() => untrack(() => workspace.open()));
  $effect(() => untrack(() => containers.start()));
  $effect(() => {
    const wanted = targetId;
    if (!wanted) return;
    void untrack(() => backup.readArchives(wanted));
  });

  const targets = $derived(backup.all);
  const archives = $derived(targetId ? backup.archivesAt(targetId) : { status: 'idle' as const });
  const watch = $derived<Watch | undefined>(watchedJob ? jobs.of(watchedJob) : undefined);
  const mayStructure = $derived(mayHere('STRUCTURE').status === 'permitted');

  // A restore finishing is a status change nobody focused (4.1.3): said once, when it ends.
  $effect(() => {
    const ended = watch;
    if (!ended || ended.watching) return;
    if (ended.job.status === 'SUCCEEDED') announcer.say(t('app.restore.finished_announced'));
    else if (ended.job.status === 'FAILED' || ended.job.status === 'CANCELLED') {
      announcer.say(sentence(ended.job.error_code) ?? t('app.restore.failed'));
    }
  });
  /** Everything a request is made of, as one comparable value. */
  const signature = $derived(
    [targetId, archiveId, mode, conflictRule, [...chosen].sort().join(','), String(safetyBackup), passphrase].join('|'),
  );
  const rehearsal = $derived<Run | undefined>(rehearsalId && signature === rehearsedFrom ? restores.of(rehearsalId) : undefined);
  const real = $derived<Run | undefined>(realId ? restores.of(realId) : undefined);
  const name = $derived(workspace.workspace?.display_name ?? '');
  const chosenMode = $derived(mode as Mode);
  const destructive = $derived(isDestructive(chosenMode));

  /**
   * What the rehearsal still needs, in the order the steps ask it: a target, an archive, a
   * selection, and for a destructive mode the three doors. Never a switched-off button: pressed,
   * each missing one is marked and focus goes to the first (UC-WRK-32 check 3).
   */
  function requirements(): Requirement[] {
    return [
      { kind: 'field', key: 'target', code: 'app.restore.target_needed', isFilled: targetId !== '' },
      { kind: 'field', key: 'archive', code: 'app.restore.archive_needed', isFilled: targetId === '' || archiveId !== '' },
      { kind: 'at-least-one', key: 'selection', code: 'app.restore.selection_needed', chosen: chosenMode === 'SELECTIVE' ? chosen.length : 1 },
      { kind: 'field', key: 'cost', code: 'app.restore.cost_needed', isFilled: !destructive || costAcknowledged },
      { kind: 'field', key: 'name', code: 'app.restore.name_needed', isFilled: !destructive || (typedName.trim() === name && name !== '') },
      { kind: 'field', key: 'safety', code: 'app.restore.safety_needed', isFilled: !destructive || safetyBackup || safetyOffAcknowledged },
    ];
  }
  let missing = $state<Missing>({});
  let form = $state<HTMLFormElement | undefined>(undefined);
  /** A field's mark while its requirement is still unmet. */
  const markOf = (key: string): string | undefined => (missing[key] && key in check(requirements()) ? t(missing[key]) : undefined);

  const when = (at: string | null | undefined) => (at ? formatDateTime(at, messages.locale) : undefined);
  const sentence = (code: string | null | undefined) => (code ? (messages.has(code) ? t(code) : code) : undefined);
  const modeWord = (which: string) => t(`app.restore.mode_${which.toLowerCase()}`);

  /**
   * The archive the picker names, found by its **path**: `RestoreRequest.archive_id` is the path at
   * the target rather than the manifest's `archive_id`, because a restore has to be possible when
   * the database that recorded the run is gone. `hubctl restore` takes the same value.
   */
  const chosenArchive = $derived<Archive | undefined>(
    archives.status === 'ready' ? archives.data.find((archive) => archive.path === archiveId) : undefined,
  );

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

  /** What this attempt is asking for, as one value, so the real run is the rehearsal again. */
  function compose(): Asked {
    return {
      target_id: targetId,
      archive_id: archiveId,
      mode: chosenMode,
      // Always this workspace: the two modes that mint or cross a tenant are not offered, and the
      // server refuses a request that does not name one.
      ...(workspace.workspace?.id ? { target_tenant_id: workspace.workspace.id } : {}),
      ...(chosenMode === 'SELECTIVE' ? { selection: { container_ids: chosen } } : {}),
      conflict_rule: conflictRule as Asked['conflict_rule'],
      ...(destructive ? { create_safety_backup: safetyBackup, confirmation: typedName.trim() } : {}),
      ...(passphrase !== '' ? { decryption_passphrase: passphrase } : {}),
    };
  }

  /** The rehearsal. Writes nothing, and is the only way to reach the second button. */
  async function rehearse(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    missing = check(requirements());
    if (!isComplete(missing)) {
      const first = Object.keys(missing)[0];
      step = (first && STEP_OF[first]) || step;
      void tick().then(() => focusFirst(form));
      return;
    }
    await attempt(async () => {
      const request = compose();
      // The proof is demanded for a destructive mode on the dry run too - the server decides, and
      // `stepUp.around` turns its refusal into the prompt rather than a dead end.
      const accepted = await stepUp.around((token) => restores.start(request, true, token));
      asked = request;
      realId = '';
      rehearsedFrom = signature;
      rehearsalId = restores.idOf(accepted) ?? '';
      watchedJob = accepted.job_id;
      step = 'confirm';
    });
  }

  /** The same request, without `dry_run` - `asked`, not `compose()`: what was reviewed runs. */
  async function runForReal(): Promise<void> {
    const request = asked;
    if (!request) return;
    await attempt(async () => {
      const accepted = await stepUp.around((token) => restores.start(request, false, token));
      realId = restores.idOf(accepted) ?? '';
      watchedJob = accepted.job_id;
    }, t('app.restore.started_announced'));
  }

  // A finished job means the report has arrived. Read it once, rather than polling for it.
  $effect(() => {
    const current = watch;
    if (!current || current.watching || !isTerminal(current.job.status)) return;
    const rehearsed = rehearsalId;
    const done = realId;
    void untrack(async () => {
      if (rehearsed) await restores.read(rehearsed);
      if (done) await restores.read(done);
    });
  });

  const title = $derived(t('app.restore.title'));
  const stepViews = $derived(STEPS.map((each, index) => ({ id: each, label: t('app.restore.step', { n: String(index + 1), name: t(`app.restore.step_${each}`) }) })));
  const next = $derived(STEPS[STEPS.indexOf(step) + 1]);

  $effect(() => page.entitle(title));
</script>

{#snippet fact(label: string, value: string)}
  <SettingRow {label} fieldWidth="lg">
    {#snippet control()}<span class="value">{value}</span>{/snippet}
  </SettingRow>
{/snippet}

<CreatePage
  head={{
    title,
    subtitle: t('app.restore.intro'),
    breadcrumb: adminTrail({ id: 'restore', label: title }),
    primary: mayStructure ? { label: t('app.restore.rehearse'), onclick: () => form?.requestSubmit() } : undefined,
  }}
>
  {#snippet steps()}
    <div class="steps">
      <ViewSwitcher label={t('app.restore.steps')} views={stepViews} selected={step} onselect={(id) => (step = id as Step)} />
    </div>
  {/snippet}

  <!-- Stated rather than left as an absence: a control that is simply missing tells a reader
       nothing about why, and the alternative is somebody's to know. -->
  {#if step === 'archive'}
    <Banner tone="info" title={t('app.restore.operators_title')}>{t('app.restore.operators_note')}</Banner>
  {/if}

  {#if failure}
    <FailureNote problem={failure}>
      {#if failure.fields.size > 0}
        <Stack gap="050">{#each [...failure.fields] as [path, message] (path)}<span>{message}</span>{/each}</Stack>
      {/if}
    </FailureNote>
  {/if}

  <WorkspaceGate permission="STRUCTURE">
    {#snippet actions()}
      <form bind:this={form} onsubmit={rehearse}>
        <FormLayout place="page">
          {#if step === 'archive'}
            <FormSection title={t('app.restore.what_title')}>
              <SettingRow label={t('app.restore.target')} fieldWidth="md" isRequired>
                {#snippet control(nameId)}
                  <Select
                    label={t('app.restore.target')}
                    isLabelHidden
                    aria-labelledby={nameId}
                    bind:value={targetId}
                    placeholder={t('app.restore.choose_target')}
                    options={targets.map((target) => ({ value: target.id, label: target.name }))}
                    error={markOf('target')}
                  />
                {/snippet}
              </SettingRow>
              {#if targetId}
                {#if archives.status === 'loading'}
                  <p class="note waiting"><Spinner label={t('app.restore.reading_archives')} /> <span>{t('app.restore.reading_archives')}</span></p>
                {:else if archives.status === 'ready'}
                  <!-- Composed from the listing at the target rather than typed: an archive
                       identifier nobody can check is one somebody mistypes. -->
                  <SettingRow
                    label={t('app.restore.archive')}
                    help={chosenArchive
                      ? [
                          chosenArchive.path,
                          chosenArchive.size_bytes ? formatBytes(chosenArchive.size_bytes, messages.locale) : '',
                          chosenArchive.item_count !== undefined ? t('app.restore.archive_items', { count: String(chosenArchive.item_count) }) : '',
                        ].filter(Boolean).join(' · ')
                      : undefined}
                    fieldWidth="lg"
                    isRequired
                  >
                    {#snippet control(nameId)}
                      <Select
                        label={t('app.restore.archive')}
                        isLabelHidden
                        aria-labelledby={nameId}
                        bind:value={archiveId}
                        error={markOf('archive')}
                        placeholder={t('app.restore.choose_archive')}
                        options={archives.data.map((archive) => ({ value: archive.path, label: `${when(archive.created_at) ?? archive.path} · ${archive.mode}` }))}
                      />
                    {/snippet}
                  </SettingRow>
                  {#if chosenArchive}
                    {#if chosenArchive.complete === false}
                      <p class="note warning">{t('app.restore.archive_incomplete')}</p>
                    {/if}
                    <SettingRow label={t('app.restore.passphrase')} help={t('app.restore.passphrase_hint')} fieldWidth="md">
                      {#snippet control(nameId)}
                        <Input label={t('app.restore.passphrase')} isLabelHidden aria-labelledby={nameId} type="password" autocomplete="off" bind:value={passphrase} />
                      {/snippet}
                    </SettingRow>
                  {/if}
                {:else if archives.status === 'failed'}
                  <p class="note failure" role="alert">{renderProblem(archives.error, messages).message}</p>
                {/if}
              {/if}
            </FormSection>
          {:else if step === 'plan'}
            <FormSection title={t('app.restore.plan_title')}>
              <SettingRow label={t('app.restore.mode')} help={t(`app.restore.mode_${mode.toLowerCase()}_note`)} fieldWidth="md">
                {#snippet control(nameId)}
                  <Select label={t('app.restore.mode')} isLabelHidden aria-labelledby={nameId} bind:value={mode} options={modes.map((which) => ({ value: which, label: modeWord(which) }))} />
                {/snippet}
              </SettingRow>
              {#if mode === 'SELECTIVE'}
                <SettingRow label={t('app.restore.selection')} help={t('app.restore.selection_note')} fieldWidth="lg" isRequired>
                  {#snippet control(nameId)}
                    <fieldset class="selection" aria-labelledby={nameId} data-invalid={markOf('selection') ? '' : undefined} aria-describedby={markOf('selection') ? 'restore-selection-error' : undefined}>
                      {#if markOf('selection')}<p class="failure" id="restore-selection-error">{markOf('selection')}</p>{/if}
                      <!-- Composed from what is here rather than typed: an identifier somebody
                           types is one somebody mistypes. -->
                      {#each containers.hubs as container (container.id)}
                        <Checkbox
                          label={container.name}
                          checked={chosen.includes(container.id)}
                          onchange={(event) => {
                            const wanted = (event.currentTarget as HTMLInputElement).checked;
                            chosen = wanted ? [...chosen, container.id] : chosen.filter((id) => id !== container.id);
                          }}
                        />
                      {/each}
                    </fieldset>
                  {/snippet}
                </SettingRow>
              {/if}
              {#if mode === 'SELECTIVE' || mode === 'MERGE'}
                <SettingRow label={t('app.restore.conflict_rule')} help={t('app.restore.conflict_rule_hint')} fieldWidth="md">
                  {#snippet control(nameId)}
                    <Select
                      label={t('app.restore.conflict_rule')}
                      isLabelHidden
                      aria-labelledby={nameId}
                      bind:value={conflictRule}
                      options={CONFLICT_RULES.map((rule) => ({ value: rule, label: t(`app.restore.conflict_${rule.toLowerCase()}`) }))}
                    />
                  {/snippet}
                </SettingRow>
              {/if}
            </FormSection>
            {#if !mayRunForReal(chosenMode, holdInForce)}
              <!-- Said before the rehearsal rather than after it: the replace will not run, and why. -->
              <Banner tone="warning" title={t('app.restore.replace_held_title')}>{t('app.restore.replace_held')}</Banner>
            {/if}
          {:else}
            <FormSection title={t('app.restore.summary_title')}>
              {@render fact(t('app.restore.target'), targets.find((target) => target.id === targetId)?.name ?? t('app.restore.not_chosen'))}
              {@render fact(t('app.restore.archive'), chosenArchive ? `${when(chosenArchive.created_at) ?? chosenArchive.path} · ${chosenArchive.mode}` : t('app.restore.not_chosen'))}
              {@render fact(t('app.restore.mode'), modeWord(mode))}
            </FormSection>
            {#if destructive}
              <!-- The first door, and the sentence that has to be read before the other two: a
                   fact about the operation, so the catalogue carries it. -->
              <Banner tone="danger" title={t('app.restore.replace_title')}>
                <Stack gap="100">
                  <span>{t('app.restore.replace_note')}</span>
                  <span>{t('app.restore.credentials_note')}</span>
                </Stack>
              </Banner>
              <FormSection title={t('app.restore.doors_title')}>
                <SettingRow label={t('app.restore.acknowledge_cost')} fieldWidth="sm" isRequired>
                  {#snippet control(nameId)}
                    <Checkbox label={t('app.restore.acknowledge_cost')} isLabelHidden aria-labelledby={nameId} bind:checked={costAcknowledged} error={markOf('cost')} />
                  {/snippet}
                </SettingRow>
                <SettingRow label={t('app.restore.type_name', { name })} help={t('app.restore.type_name_hint')} fieldWidth="md" isRequired>
                  {#snippet control(nameId)}
                    <Input label={t('app.restore.type_name', { name })} isLabelHidden aria-labelledby={nameId} bind:value={typedName} error={markOf('name')} />
                  {/snippet}
                </SettingRow>
                <SettingRow label={t('app.restore.safety_backup')} help={t('app.restore.safety_backup_hint')} fieldWidth="sm">
                  {#snippet control(nameId)}
                    <Switch label={t('app.restore.safety_backup')} isLabelHidden aria-labelledby={nameId} bind:checked={safetyBackup} />
                  {/snippet}
                </SettingRow>
              </FormSection>
              {#if !safetyBackup}
                <!-- Its own confirmation, naming what it gives up: turning the copy off is a second
                     decision, not a side effect of the first. -->
                <Banner tone="danger" title={t('app.restore.no_safety_title')}>
                  <Stack gap="100">
                    <span>{t('app.restore.no_safety_note')}</span>
                    <Checkbox label={t('app.restore.acknowledge_no_safety')} bind:checked={safetyOffAcknowledged} error={markOf('safety')} />
                  </Stack>
                </Banner>
              {/if}
            {/if}
            <p class="quiet">{t('app.restore.rehearse_note')}</p>
          {/if}
        </FormLayout>
        {#if next}
          <div class="next"><Button tone="secondary" onclick={() => (step = next)}>{t('app.restore.next', { name: t(`app.restore.step_${next}`) })}</Button></div>
        {/if}
      </form>
    {/snippet}
  </WorkspaceGate>

  {#if step === 'confirm'}
    {#if watch}
      <section class="region" aria-label={realId ? t('app.restore.running_title') : t('app.restore.rehearsing_title')}>
        <Stack gap="100">
          <h2 class="title">{realId ? t('app.restore.running_title') : t('app.restore.rehearsing_title')}</h2>
          {#if watch.watching}
            <ProgressBar
              label={realId ? t('app.restore.running_title') : t('app.restore.rehearsing_title')}
              value={watch.job.progress === null || watch.job.progress === undefined ? undefined : Math.round(watch.job.progress * 100)}
              valueLabel={watch.job.progress === null || watch.job.progress === undefined
                ? t('app.restore.progress_unknown')
                : t('app.restore.progress_share', { percent: String(Math.round(watch.job.progress * 100)) })}
            />
            {#if mayCancel(watch.job.status)}
              <div>
                <Button size="sm" tone="secondary" onclick={() => void attempt(() => jobs.cancel(watch!.job.job_id), t('app.jobs.cancel_asked_announced'))}>
                  {t('app.restore.cancel')}
                </Button>
              </div>
            {/if}
          {:else if watch.unreachable}
            <p class="failure" role="alert">{sentence(watch.unreachable)}</p>
          {:else if watch.job.status !== 'SUCCEEDED'}
            <p class="failure" role="alert">{sentence(watch.job.error_code) ?? t('app.restore.failed')}</p>
          {/if}
        </Stack>
      </section>
    {/if}

    {#if rehearsal}
      <section class="region" aria-label={t('app.restore.report_title')}>
        <Stack gap="150">
          <div class="line">
            <h2 class="title">{t('app.restore.report_title')}</h2>
            <Badge tone="info">{t('app.restore.was_a_rehearsal')}</Badge>
          </div>
          {#if rehearsal.status !== 'SUCCEEDED'}
            <!-- A rehearsal that did not finish has no report; an empty one read as "nothing would
                 change" would call a restore safe because the archive could not be read. -->
            <Banner tone="danger" title={t('app.restore.rehearsal_failed')}>{sentence(rehearsal.error_code) ?? t('app.restore.failed')}</Banner>
          {/if}
          <p class="quiet">{modeWord(rehearsal.mode)} · {rehearsal.source_archive}</p>
          {#if rehearsal.status === 'SUCCEEDED'}<RestoreReport report={rehearsal.report} />{/if}
          <p class="quiet">{t('app.restore.rehearsal_wrote_nothing')}</p>
          {#if !realId && rehearsal.status === 'SUCCEEDED' && mayRunForReal(rehearsal.mode as Mode, holdInForce) && (!isDestructive(rehearsal.mode as Mode) || mayHere('DELETE_CONTAINER').status === 'permitted')}
            <!-- The second request, composed from the first. Offered only once a report exists. -->
            <div>
              <Button tone="danger" isBusy={isWorking} busyLabel={t('app.restore.running')} onclick={() => void runForReal()}>
                {t('app.restore.run_for_real')}
              </Button>
            </div>
            <p class="quiet">{t('app.restore.run_for_real_note')}</p>
          {/if}
        </Stack>
      </section>
    {/if}

    {#if real}
      <section class="region" aria-label={t('app.restore.done_title')}>
        <Stack gap="150">
          <div class="line">
            <h2 class="title">{t('app.restore.done_title')}</h2>
            <Badge tone={real.status === 'SUCCEEDED' ? 'success' : 'danger'}>{real.status}</Badge>
          </div>
          <RestoreReport report={real.report} />
          {#if real.safety_backup_run_id}
            <!-- The way back is a run identifier rather than a search at the target. -->
            <p class="quiet">{t('app.restore.safety_taken', { id: real.safety_backup_run_id })}</p>
          {/if}
          {#if real.error_code}<p class="failure" role="alert">{sentence(real.error_code)}</p>{/if}
        </Stack>
      </section>
    {/if}
  {/if}
</CreatePage>

<style>
  form { margin: 0; display: flex; flex-direction: column; gap: var(--sp-250); }
  .steps { display: flex; }
  .next { display: flex; justify-content: flex-end; }
  .line { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .title { margin: 0; font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .value { color: var(--text-primary); overflow-wrap: anywhere; }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .warning { color: var(--text-warning); font-size: var(--fs-075); }
  /* A line inside a region of setting rows, on the rows' own gutter. */
  .note { margin: 0; padding: var(--sp-150) var(--sp-250); border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .waiting { display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .selection { margin: 0; padding: 0; border: none; display: grid; gap: var(--sp-050); }
  .region {
    padding: var(--sp-200) var(--sp-250);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
</style>
