<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One privacy case as its record page (page-system §7.2, §9.3): who it is about, the right asked
  // for, its state and its deadline in the head with the next step - *Start* or *Complete* - as the
  // head's act; what it holds and, for an erasure, how it erases as regions; *Extend the deadline…*
  // (Dialog M) and *Reject…* (Dialog S) in the head's ⋯.
  //
  // **`erasure_mode` defaults to `ANONYMIZE`, and the page says what each mode costs before the
  // choice is made** (P-6): tenant data touches third parties' rights. Nothing here ever defaults a
  // case to `FULL_DELETE`. **An archive goes to a backup target** like every other export, so a
  // finished access case reports where it was written rather than offering a download (ADR-0047).
  // A case the operator recorded at the installation carries no control that moves it (`canAct`).

  import { untrack } from 'svelte';

  import { EmptyState, RecordPage, Select, Stack, type RecordList } from '@hubtask/design-system/components';
  import type { ErasureKept } from '@hubtask/sync-engine';
  import type { MenuItem } from '@hubtask/design-system/components';

  import { announcer } from '../lib/announce.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { backup } from '../lib/data/backup.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import {
    canAct, canExtend, canStart, confirmsStart, deadlinePhrase, extensionPhrase, keptPartPhrase, keptPhrase, privacy, producesArchive, standingOf,
    type ErasureMode, type Request,
  } from '../lib/data/privacy.svelte.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { formatRelative } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import ConfirmEraseDialog from '../lib/privacy/ConfirmEraseDialog.svelte';
  import ExtendDialog from '../lib/privacy/ExtendDialog.svelte';
  import RejectDialog from '../lib/privacy/RejectDialog.svelte';
  import { day, kindWord, statusTone, statusWord, subjectOf, when } from '../lib/privacy/words.svelte.ts';
  import ErasureLines from '../lib/workspace/ErasureLines.svelte';

  interface Props {
    id: string;
    onnavigate: (path: string) => void;
  }

  const { id, onnavigate }: Props = $props();

  const LIST = '/administration/privacy';
  const MODES: readonly ErasureMode[] = ['ANONYMIZE', 'FULL_DELETE'];

  // A case may be closed: read the list with the closed ones.
  $effect(() => untrack(() => privacy.open(true)));
  $effect(() => untrack(() => backup.open()));
  $effect(() => untrack(() => workspace.open()));

  const request = $derived<Request | undefined>(privacy.of(true).find((each) => each.id === id) ?? privacy.of(false).find((each) => each.id === id));
  const reading = $derived(privacy.stateOf(true));
  $effect(() => {
    const current = request;
    if (current) untrack(() => accounts.resolve([current.subject_account_id, current.handled_by]));
  });

  const title = $derived(request ? subjectOf(request) : t('app.privacy.case_kind'));
  $effect(() => page.entitle(title));

  const reader = $derived({ mayManage: holdsHere('MANAGE_MEMBERS') === true, mayDestroy: holdsHere('DELETE_CONTAINER') === true });
  const now = $derived.by(() => Date.now());

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  let confirming = $state<Request | undefined>(undefined);
  let extending = $state<Request | undefined>(undefined);
  let rejecting = $state<Request | undefined>(undefined);

  async function attempt(work: () => Promise<unknown>, said: string): Promise<void> {
    failure = undefined;
    isWorking = true;
    try {
      await work();
      announcer.say(said);
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }

  const isOpenCase = $derived(request !== undefined && (request.status === 'RECEIVED' || request.status === 'IN_PROGRESS'));
  const mayMove = $derived(request !== undefined && canAct(request, reader) && isOpenCase);

  /** The next step as the head's act: start a received case, complete one in progress. */
  const next = $derived.by(() => {
    if (!request || !mayMove) return undefined;
    if (request.status === 'RECEIVED') {
      if (!canStart(request, reader)) return undefined;
      const current = request;
      return {
        label: t('app.privacy.start'),
        isBusy: isWorking,
        busyLabel: t('app.privacy.starting'),
        onclick: () => (confirmsStart(current) ? (confirming = current) : void attempt(() => privacy.change(current.id, { status: 'IN_PROGRESS' }), t('app.privacy.started_announced'))),
      };
    }
    const current = request;
    return {
      label: t('app.privacy.complete'),
      isBusy: isWorking,
      busyLabel: t('app.privacy.completing'),
      onclick: () => void attempt(() => privacy.change(current.id, { status: 'COMPLETED' }), t('app.privacy.completed_announced')),
    };
  });

  const menuItems = $derived<MenuItem[]>(
    request && mayMove
      ? [
          ...(canExtend(request, reader) ? [{ id: 'extend', label: t('app.privacy.extend_open') }] : []),
          { id: 'reject', label: t('app.privacy.reject_open'), isDestructive: true, hasSeparatorBefore: true },
        ]
      : [],
  );

  const facts = $derived.by(() => {
    if (!request) return undefined;
    const deadline = deadlinePhrase(request, (at) => when(at) ?? '');
    const standing = standingOf(request, now);
    return [
      kindWord(request.kind),
      t(deadline.code, deadline.params) + (standing ? ` (${formatRelative(request.due_at, messages.locale, now)})` : ''),
      t('app.privacy.received', { at: request.received_on ? day(request.received_on) : (when(request.received_at) ?? '') }),
      request.handled_by ? t('app.privacy.handled_by', { name: accounts.nameOf(request.handled_by) ?? t('app.people.unnamed') }) : undefined,
    ].filter(Boolean).join(' · ');
  });

  const lists = $derived<RecordList[]>([
    { id: 'case', title: t('app.privacy.case_about') },
    ...(request?.kind === 'ERASURE' ? [{ id: 'erasure', title: t('app.privacy.erasure_title') }] : []),
  ]);

  const targetName = (target: string | null | undefined) => (target ? backup.all.find((each) => each.id === target)?.name : undefined) ?? target ?? '';
  const scopeWord = (part: ErasureKept) => t(`app.privacy.hold_scope_${part.hold_scope.kind.toLowerCase()}`);
  const standing = $derived(request ? standingOf(request, now) : undefined);
</script>

<RecordPage
  record={{
    kind: t('app.privacy.case_kind'),
    title,
    back: { label: t('app.privacy.title'), href: LIST, onnavigate: () => onnavigate(LIST) },
    status: request
      ? standing === 'overdue'
        ? { label: t('app.privacy.overdue'), tone: 'danger' }
        : standing === 'soon'
          ? { label: t('app.privacy.due_soon'), tone: 'warning' }
          : { label: statusWord(request.status), tone: statusTone(request.status) }
      : undefined,
    facts,
    edit: next,
    menu: menuItems.length > 0
      ? { label: t('app.page.more_for', { name: title }), items: menuItems, onselect: (item: string) => { if (item === 'extend') extending = request; else rejecting = request; } }
      : undefined,
  }}
  {lists}
  refused={reading.status === 'failed' || (reading.status === 'ready' && !request) ? notHere : undefined}
>
  {#snippet list(item: RecordList)}
    {#if request && item.id === 'case'}
      <Stack gap="100">
        {#if failure}<FailureNote problem={failure} />{/if}
        <p class="line"><span class="label">{t('app.privacy.status_column')}</span> {statusWord(request.status)}</p>
        {#if extensionPhrase(request, day)}
          {@const extension = extensionPhrase(request, day)}
          <p class="quiet">{extension ? t(extension.code, extension.params) : ''}</p>
        {/if}
        <!-- What this kind means, said where the case is read. -->
        <p class="quiet">{t(`app.privacy.kind_${request.kind.toLowerCase()}_note`)}</p>
        {#if request.notes}<p>{request.notes}</p>{/if}
        {#if request.result_archive}
          <!-- Where it went. No download (ADR-0047). -->
          <p class="quiet">{t('app.privacy.archive_at', { target: targetName(request.result_target_id), path: request.result_archive })}</p>
        {/if}
        {#if keptPhrase(request)}
          {@const kept = keptPhrase(request)}
          <!-- Partly completed: what a hold kept, and why it may (UC-PRV-03 check 11). -->
          <p class="quiet">{kept ? t(kept.code, { ...kept.params, at: kept.params.at ? (when(String(kept.params.at)) ?? '') : '' }) : ''}</p>
          {#each (request.kept ?? []).filter((part) => !part.erased_at) as part (part.hold_id)}
            {@const line = keptPartPhrase(part, scopeWord(part))}
            <p class="quiet">{t(line.code, line.params)}</p>
          {/each}
        {/if}
        {#if request.rejection_reason}<p class="quiet">{t('app.privacy.rejected_because', { reason: request.rejection_reason })}</p>{/if}
        {#if request.status === 'RECEIVED' && mayMove && producesArchive(request.kind)}
          <!-- Starting is what runs the work, and the work writes an archive somewhere. -->
          <p class="quiet">{t('app.privacy.start_makes_archive')}</p>
        {/if}
      </Stack>
    {:else if request && item.id === 'erasure'}
      <Stack gap="100">
        {#if canAct(request, reader) && request.status !== 'COMPLETED'}
          <!-- What each mode does to other people's content, before the choice is made. -->
          <div class="narrow">
            <Select
              label={t('app.privacy.erasure_mode')}
              value={request.erasure_mode ?? 'ANONYMIZE'}
              options={MODES.map((mode) => ({ value: mode, label: t(`app.privacy.mode_${mode.toLowerCase()}`) }))}
              onchange={(event) =>
                void attempt(() => privacy.change(request.id, { erasure_mode: (event.currentTarget as HTMLSelectElement).value as ErasureMode }), t('app.privacy.mode_changed_announced'))}
            />
          </div>
        {:else}
          <p>{t(`app.privacy.mode_${(request.erasure_mode ?? 'ANONYMIZE').toLowerCase()}`)}</p>
        {/if}
        <p class="quiet">{t(`app.privacy.mode_${(request.erasure_mode ?? 'ANONYMIZE').toLowerCase()}_note`)}</p>
        <ErasureLines requestId={request.id} mode={request.erasure_mode ?? 'ANONYMIZE'} />
      </Stack>
    {/if}
  {/snippet}
</RecordPage>

<ConfirmEraseDialog request={confirming} onClose={() => (confirming = undefined)} />
<ExtendDialog request={extending} onClose={() => (extending = undefined)} />
<RejectDialog request={rejecting} onClose={() => (rejecting = undefined)} />

{#snippet notHere()}
  {#if reading.status === 'failed'}
    <FailureNote problem={renderProblem(reading.error, messages)} />
  {:else}
    <EmptyState kind="settled" title={t('app.privacy.case_gone')} />
  {/if}
{/snippet}

<style>
  p { margin: 0; }
  .quiet { color: var(--text-secondary); }
  .label { color: var(--text-secondary); margin-inline-end: var(--sp-100); }
  .narrow { max-inline-size: var(--layout-field-md); }
</style>
