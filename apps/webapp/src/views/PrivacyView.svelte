<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The screen a controller answers a person's request from (`data-protection.md` §4, §9, §12).
  //
  // **The deadline is the column that matters**, so the list is ordered by what is closest to it
  // and marked the way the product marks a due date everywhere else: an icon *and* a word beside
  // the date, never colour alone (`design-system.md` §6, rule 3). A statutory deadline is a due
  // date, and a second visual language for it would be a design decision nobody took.
  //
  // **`erasure_mode` defaults to `ANONYMIZE`, and the screen says what each mode costs before the
  // choice is made.** P-6's decision, because tenant data touches third parties' rights: a task
  // somebody else depends on, a comment in a thread that stops making sense without it. Nothing
  // here ever defaults a case to `FULL_DELETE`.
  //
  // **`INSTALLATION` scope is not offered** and the alternative is named: it crosses the tenant
  // boundary and is the provider's path. It is absent from the types, so nothing here can compose
  // one.
  //
  // **An archive goes to a backup target like every other export**, so a finished access case
  // reports where it was written rather than offering a download (ADR-0047).

  import { untrack } from 'svelte';

  import {
    Badge,
    Banner,
    Button,
    Input,
    Select,
    Spinner,
    Stack,
    Textarea,
  } from '@hubtask/design-system/components';

  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { backup } from '../lib/data/backup.svelte.ts';
  import {
    byDeadline,
    KINDS,
    privacy,
    producesArchive,
    standingOf,
    type ErasureMode,
    type Kind,
    type Request,
  } from '../lib/data/privacy.svelte.ts';
  import { formatDateTime, formatRelative } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  const MODES: readonly ErasureMode[] = ['ANONYMIZE', 'FULL_DELETE'];

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  let includeClosed = $state(false);

  let draftKind = $state<string>('ACCESS');
  let draftEmail = $state('');
  let draftAccount = $state('');
  let draftTarget = $state('');
  let draftNotes = $state('');

  let rejecting = $state('');
  let rejectReason = $state('');

  let restrictAccount = $state('');
  let restrictReason = $state('');
  let withdrawPurpose = $state('');
  let withdrawAccount = $state('');

  $effect(() => {
    const closed = includeClosed;
    return untrack(() => privacy.open(closed));
  });
  $effect(() => untrack(() => backup.open()));

  const reading = $derived(privacy.stateOf(includeClosed));
  const listed = $derived(byDeadline(privacy.of(includeClosed)));
  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  // Read once per render rather than held, for `DueMark`'s reason: nothing here animates, and a
  // timer that re-rendered every row every minute would burn a battery to move one word.
  const now = $derived.by(() => Date.now());

  $effect(() => {
    const rows = listed;
    // Both identifiers a row shows: who the case is about, and who is carrying it. Resolving one
    // and not the other is how a row comes to name a person beside an identifier.
    untrack(() =>
      accounts.resolve([
        ...rows.map((row) => row.subject_account_id),
        ...rows.map((row) => row.handled_by),
      ]),
    );
  });

  const when = (at: string | null | undefined) =>
    at ? formatDateTime(at, messages.locale) : undefined;

  const kindWord = (kind: string) => t(`app.privacy.kind_${kind.toLowerCase()}`);
  const statusWord = (status: string) => t(`app.privacy.status_${status.toLowerCase()}`);

  /** Who the case is about: the resolved name, the address it named, or nobody nameable. */
  const subjectOf = (request: Request) =>
    accounts.nameOf(request.subject_account_id) ??
    request.subject_email ??
    t('app.privacy.subject_unknown');

  async function attempt(work: () => Promise<unknown>): Promise<void> {
    failure = undefined;
    isWorking = true;
    try {
      await work();
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }

  async function record(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!draftEmail.trim() && !draftAccount.trim()) return;
    await attempt(async () => {
      await privacy.record({
        kind: draftKind as Kind,
        ...(draftAccount.trim() ? { subject_account_id: draftAccount.trim() } : {}),
        ...(draftEmail.trim() ? { subject_email: draftEmail.trim() } : {}),
        ...(draftTarget ? { target_id: draftTarget } : {}),
        ...(draftNotes.trim() ? { notes: draftNotes.trim() } : {}),
      });
      draftEmail = '';
      draftAccount = '';
      draftNotes = '';
    });
  }

  const targetName = (id: string | null | undefined) =>
    (id ? backup.all.find((target) => target.id === id)?.name : undefined) ?? id ?? '';
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.privacy.title')}</h1>
    <p class="quiet">{t('app.privacy.intro')}</p>

    <!-- Stated rather than left as an absence: the alternative is somebody's to know. -->
    <Banner tone="info" title={t('app.privacy.installation_title')}>
      {t('app.privacy.installation_note')}
    </Banner>

    {#if failure}
      <Banner tone="danger" title={failure.message}>
        <Stack gap="050">
          {#each [...failure.fields] as [path, message] (path)}
            <span>{message}</span>
          {/each}
          {#if failure.reference}
            <span>{t('app.error_reference', { request_id: failure.reference })}</span>
          {/if}
        </Stack>
      </Banner>
    {/if}

    <Stack gap="150">
      <div class="row">
        <h2 class="section">{t('app.privacy.cases_title')}</h2>
        <Button size="sm" tone="subtle" onclick={() => (includeClosed = !includeClosed)}>
          {includeClosed ? t('app.privacy.hide_closed') : t('app.privacy.show_closed')}
        </Button>
      </div>

      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="waiting">
          <Spinner label={t('app.privacy.reading')} /> <span>{t('app.privacy.reading')}</span>
        </p>
      {:else if refusal}
        <Banner tone="danger" title={refusal.message}>
          {#if refusal.reference}{t('app.error_reference', { request_id: refusal.reference })}{/if}
        </Banner>
      {:else}
        {#each listed as request (request.id)}
          {@const standing = standingOf(request, now)}
          <section class="panel">
            <Stack gap="100">
              <div class="row">
                <Badge>{kindWord(request.kind)}</Badge>
                <Badge
                  tone={request.status === 'COMPLETED'
                    ? 'success'
                    : request.status === 'REJECTED'
                      ? 'neutral'
                      : 'info'}
                >
                  {statusWord(request.status)}
                </Badge>
                <span class="name">{subjectOf(request)}</span>
              </div>

              <!-- The deadline, marked the way the product marks a due date: the icon and the word
                   carry it as well as the colour does. -->
              <p class="line">
                {#if standing === 'overdue'}
                  <Badge tone="danger" icon="triangle-alert">{t('app.privacy.overdue')}</Badge>
                {:else if standing === 'soon'}
                  <Badge tone="warning" icon="clock">{t('app.privacy.due_soon')}</Badge>
                {/if}
                <span class="quiet small">
                  {t('app.privacy.due', { at: when(request.due_at) ?? '' })}
                  {#if standing}
                    · {formatRelative(request.due_at, messages.locale, now)}
                  {/if}
                </span>
              </p>

              <p class="quiet small">
                {t('app.privacy.received', { at: when(request.received_at) ?? '' })}
                {#if request.handled_by}
                  · {t('app.privacy.handled_by', {
                    name: accounts.nameOf(request.handled_by) ?? t('app.people.unnamed'),
                  })}
                {/if}
              </p>

              <!-- What this kind means, said per row rather than in a legend: six rights that each
                   mean something different are six sentences, and a legend is read once. -->
              <p class="quiet small">{t(`app.privacy.kind_${request.kind.toLowerCase()}_note`)}</p>

              {#if request.notes}<p class="quiet small">{request.notes}</p>{/if}

              {#if request.result_archive}
                <!-- Where it went. No download: the archive lies at a target the browser holds no
                     credential for (ADR-0047). -->
                <p class="quiet small">
                  {t('app.privacy.archive_at', {
                    target: targetName(request.result_target_id),
                    path: request.result_archive,
                  })}
                </p>
              {/if}

              {#if request.rejection_reason}
                <p class="quiet small">
                  {t('app.privacy.rejected_because', { reason: request.rejection_reason })}
                </p>
              {/if}

              {#if request.kind === 'ERASURE' && request.status !== 'COMPLETED'}
                <!-- What each mode does to other people's content, before the choice is made. -->
                <Stack gap="050">
                  <Select
                    label={t('app.privacy.erasure_mode')}
                    value={request.erasure_mode ?? 'ANONYMIZE'}
                    options={MODES.map((mode) => ({
                      value: mode,
                      label: t(`app.privacy.mode_${mode.toLowerCase()}`),
                    }))}
                    onchange={(event) =>
                      void attempt(() =>
                        privacy.change(request.id, {
                          erasure_mode: (event.currentTarget as HTMLSelectElement)
                            .value as ErasureMode,
                        }),
                      )}
                  />
                  <p class="quiet small">
                    {t(`app.privacy.mode_${(request.erasure_mode ?? 'ANONYMIZE').toLowerCase()}_note`)}
                  </p>
                </Stack>
              {/if}

              {#if request.status === 'RECEIVED' || request.status === 'IN_PROGRESS'}
                <div class="row">
                  {#if request.status === 'RECEIVED'}
                    <Button
                      size="sm"
                      tone="primary"
                      isBusy={isWorking}
                      busyLabel={t('app.privacy.starting')}
                      onclick={() =>
                        void attempt(() => privacy.change(request.id, { status: 'IN_PROGRESS' }))}
                    >
                      {t('app.privacy.start')}
                    </Button>
                  {:else}
                    <Button
                      size="sm"
                      tone="primary"
                      isBusy={isWorking}
                      busyLabel={t('app.privacy.completing')}
                      onclick={() =>
                        void attempt(() => privacy.change(request.id, { status: 'COMPLETED' }))}
                    >
                      {t('app.privacy.complete')}
                    </Button>
                  {/if}
                  <Button
                    size="sm"
                    tone="danger"
                    onclick={() => (rejecting = rejecting === request.id ? '' : request.id)}
                  >
                    {t('app.privacy.reject')}
                  </Button>
                </div>

                {#if request.status === 'RECEIVED' && producesArchive(request.kind)}
                  <!-- Starting is what runs the work, and the work writes an archive somewhere. -->
                  <p class="quiet small">{t('app.privacy.start_makes_archive')}</p>
                {/if}
              {/if}

              {#if rejecting === request.id}
                <Stack gap="100">
                  <Input
                    label={t('app.privacy.reject_reason')}
                    hint={t('app.privacy.reject_reason_hint')}
                    bind:value={rejectReason}
                    isRequired
                  />
                  <div class="row">
                    <Button
                      size="sm"
                      tone="danger"
                      isBusy={isWorking}
                      busyLabel={t('app.privacy.rejecting')}
                      onclick={() =>
                        void attempt(async () => {
                          await privacy.change(request.id, {
                            status: 'REJECTED',
                            rejection_reason: rejectReason.trim(),
                          });
                          rejecting = '';
                          rejectReason = '';
                        })}
                    >
                      {t('app.privacy.reject_confirm')}
                    </Button>
                    <Button size="sm" tone="subtle" onclick={() => (rejecting = '')}>
                      {t('app.workspace.cancel')}
                    </Button>
                  </div>
                </Stack>
              {/if}
            </Stack>
          </section>
        {:else}
          <p class="quiet">{t('app.privacy.none')}</p>
        {/each}

        {#if privacy.moreAfter(includeClosed)}
          <div>
            <Button tone="secondary" onclick={() => void attempt(() => privacy.more(includeClosed))}>
              {t('app.privacy.more')}
            </Button>
          </div>
        {/if}
      {/if}
    </Stack>

    <form class="panel" onsubmit={record}>
      <Stack gap="150">
        <h2 class="section">{t('app.privacy.record_title')}</h2>
        <p class="quiet small">{t('app.privacy.record_note')}</p>
        <Select
          label={t('app.privacy.kind')}
          bind:value={draftKind}
          options={KINDS.map((kind) => ({ value: kind, label: kindWord(kind) }))}
        />
        <p class="quiet small">{t(`app.privacy.kind_${draftKind.toLowerCase()}_note`)}</p>
        <Input
          label={t('app.privacy.subject_email')}
          hint={t('app.privacy.subject_email_hint')}
          type="email"
          bind:value={draftEmail}
        />
        <Input
          label={t('app.privacy.subject_account')}
          hint={t('app.privacy.subject_account_hint')}
          bind:value={draftAccount}
        />
        {#if producesArchive(draftKind as Kind)}
          <!-- Required before such a case can start: a copy of somebody's data has to be put
               somewhere, and this system writes archives to configured targets. -->
          <Select
            label={t('app.privacy.target')}
            hint={t('app.privacy.target_hint')}
            bind:value={draftTarget}
            placeholder={t('app.privacy.choose_target')}
            options={backup.all.map((target) => ({ value: target.id, label: target.name }))}
          />
        {/if}
        <Textarea label={t('app.privacy.notes')} bind:value={draftNotes} rows={2} />
        <div>
          <Button
            type="submit"
            tone="primary"
            isBusy={isWorking}
            busyLabel={t('app.privacy.recording')}
          >
            {t('app.privacy.record')}
          </Button>
        </div>
      </Stack>
    </form>

    <Stack gap="150">
      <h2 class="section">{t('app.privacy.articles_title')}</h2>
      <p class="quiet small">{t('app.privacy.articles_note')}</p>

      <form
        class="panel"
        onsubmit={(event) => {
          event.preventDefault();
          if (!restrictAccount.trim()) return;
          void attempt(async () => {
            await privacy.restrict(restrictAccount.trim(), true, restrictReason.trim() || undefined);
            restrictAccount = '';
            restrictReason = '';
          });
        }}
      >
        <Stack gap="150">
          <h3 class="section">{t('app.privacy.restrict_title')}</h3>
          <!-- What it means for the person's sessions and for the workspace's automations. -->
          <p class="quiet small">{t('app.privacy.restrict_note')}</p>
          <Input label={t('app.privacy.restrict_account')} bind:value={restrictAccount} />
          <Input label={t('app.privacy.restrict_reason')} bind:value={restrictReason} />
          <div>
            <Button
              type="submit"
              tone="secondary"
              isBusy={isWorking}
              busyLabel={t('app.privacy.restricting')}
            >
              {t('app.privacy.restrict')}
            </Button>
          </div>
        </Stack>
      </form>

      <form
        class="panel"
        onsubmit={(event) => {
          event.preventDefault();
          if (!withdrawPurpose.trim()) return;
          void attempt(async () => {
            await privacy.withdraw(withdrawPurpose.trim(), withdrawAccount.trim() || undefined);
            withdrawPurpose = '';
            withdrawAccount = '';
          });
        }}
      >
        <Stack gap="150">
          <h3 class="section">{t('app.privacy.withdraw_title')}</h3>
          <p class="quiet small">{t('app.privacy.withdraw_note')}</p>
          <Input
            label={t('app.privacy.purpose')}
            hint={t('app.privacy.purpose_hint')}
            bind:value={withdrawPurpose}
          />
          <Input
            label={t('app.privacy.withdraw_account')}
            hint={t('app.privacy.withdraw_account_hint')}
            bind:value={withdrawAccount}
          />
          <div>
            <Button
              type="submit"
              tone="secondary"
              isBusy={isWorking}
              busyLabel={t('app.privacy.withdrawing')}
            >
              {t('app.privacy.withdraw')}
            </Button>
          </div>
        </Stack>
      </form>
    </Stack>
  </Stack>
</div>

<style>
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .section {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
  }

  h3.section { font-size: var(--fs-200); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .waiting {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }

  .name { color: var(--text-primary); font-weight: var(--fw-medium); }

  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .line { margin: 0; display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
</style>
