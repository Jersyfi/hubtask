<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The calendar feeds the reader writes into their own calendar at Microsoft 365 or Google
  // (UC-INT-16), and the page the provider sends the browser back to after the sign-in.
  //
  // A push is the reader's own grant to their own calendar, so it is profile configuration like a
  // token: nobody else lists it. Connecting starts from a view's calendar subscriptions, where the
  // feed is; this screen says what is connected, whether it goes through, and takes one off.

  //
  // **A stopped push is fixed by connecting again** (user-experience.md §7.10): the provider
  // refused for good - the grant revoked or the feed gone - and connecting the same feed to the
  // same provider replaces the grant, so the act stands beside the failure it fixes.

  import { untrack } from 'svelte';

  import { Badge, Button, DataTable, EmptyState, FilterBar, ListPage, type DataColumn, type DataRowMenu } from '@hubtask/design-system/components';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import type { CalendarPush, CalendarPushList } from '@hubtask/sync-engine';

  import FinalActDialog from '../lib/frame/FinalActDialog.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { profileHead, profileTitle } from '../lib/profile/profilehead.ts';

  import { actor } from '../lib/data/account.svelte.ts';
  import { calendarPushesPath, providerAnswerOf, pushFigure } from '../lib/data/calendarpush.ts';
  import { calendarPushes } from '../lib/data/calendarpush.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    /** The query the provider sent the browser back with. */
    providerAnswer?: Record<string, string>;
    /** Called once the answer is handed on, so the address forgets the code. */
    onanswered?: () => void;
  }

  let { providerAnswer, onanswered }: Props = $props();

  const account = $derived(actor.account);
  const read = resource<CalendarPushList>(calendarPushesPath);
  const pushes = $derived(read.state.status === 'ready' ? read.state.data.data : []);
  const offered = $derived(read.state.status === 'ready' ? read.state.data.providers : []);

  let failure = $state<string | undefined>(undefined);
  let connected = $state<CalendarPush | undefined>(undefined);
  let asking = $state<CalendarPush | undefined>(undefined);
  let leaving = $state<string | undefined>(undefined);

  $effect(() => {
    const answer = untrack(() => (providerAnswer ? providerAnswerOf(providerAnswer) : undefined));
    if (!answer) return;
    void calendarPushes
      .complete(answer)
      .then((push) => (connected = push))
      .catch((error) => (failure = renderProblem(error as never, messages).message))
      .finally(() => onanswered?.());
  });

  async function connectAgain(push: CalendarPush) {
    failure = undefined;
    leaving = push.id;
    try {
      const started = await calendarPushes.authorize(push.feed_id, push.provider);
      window.location.assign(started.authorize_url);
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
      leaving = undefined;
    }
  }

  const figureText = $derived.by(() => {
    if (pushes.length === 0) return '';
    const counts = pushFigure(pushes);
    return [
      t('app.pushes.figure_active', { count: String(counts.active) }),
      ...(counts.failed > 0 ? [t('app.pushes.figure_failed', { count: String(counts.failed) })] : []),
    ].join(' · ');
  });

  const providerName = (push: CalendarPush) => t(push.provider === 'GOOGLE' ? 'app.pushes.google' : 'app.pushes.microsoft');
  let search = $state('');
  const shown = $derived(pushes.filter((push) => matchesSearch(search, [providerName(push), push.address ?? undefined])));

  const title = $derived(profileTitle('calendars'));
  $effect(() => page.entitle(title));

  const columns = $derived<DataColumn[]>([
    { id: 'calendar', label: t('app.pushes.calendar_column'), priority: 1 },
    { id: 'state', label: t('app.pushes.state_column'), priority: 1 },
    { id: 'written', label: t('app.pushes.written_column'), priority: 2 },
  ]);

  // Disconnecting is destructive: the row's ⋯ and a Dialog S (PS-6); *Connect again* is the row's
  // one quiet verb, where the push stopped and the provider is still offered.
  const rowMenu = (push: CalendarPush): DataRowMenu => ({
    label: t('app.page.more_for', { name: providerName(push) }),
    items: [{ id: 'disconnect', label: t('app.pushes.disconnect_menu'), isDestructive: true }],
  });
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <ListPage head={profileHead('calendars', { figure: figureText ? pushesFigure : undefined })}>
    {#snippet intro()}<p class="quiet">{t('app.pushes.intro')} {t('app.pushes.how')}</p>{/snippet}
    {#snippet filters()}
      <FilterBar
        searchLabel={t('app.pushes.search')}
        clearSearchLabel={t('app.page.clear_search')}
        bind:search
        countLabel={read.state.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(pushes.length) }) : undefined}
      />
    {/snippet}

    {#if connected}<p class="done" role="status">{t('app.pushes.connected', { calendar: providerName(connected) })}</p>{/if}
    {#if failure}<FailureNote problem={failure} />{/if}

    {#if read.state.status === 'ready'}
      <DataTable
        label={title}
        {columns}
        rows={shown}
        key={(push: CalendarPush) => push.id}
        rowLabel={providerName}
        menu={rowMenu}
        onmenu={(push: CalendarPush) => (asking = push)}
        actionsLabel={t('app.page.row_actions')}
        sheetDismissLabel={t('app.page.sheet_close')}
        sheetGrabLabel={t('app.page.sheet_grab')}
        shownLabel={shown.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
      >
        {#snippet cell(push: CalendarPush, column: DataColumn)}
          {#if column.id === 'calendar'}
            <span class="what" data-push={push.provider}>{providerName(push)}</span>
            {#if push.address}<span class="quiet small mono">{push.address}</span>{/if}
          {:else if column.id === 'state'}
            {#if push.state === 'ACTIVE'}
              <Badge tone="success">{t('app.pushes.state_active')}</Badge>
            {:else}
              <Badge tone="danger">{t('app.pushes.state_failed')}</Badge>
            {/if}
            <!-- Why it stopped, or why a run did not go through while it still writes: beside the
                 act that fixes it, so whoever presses it knows why. -->
            {#if push.state === 'FAILED'}
              <span class="small failed" data-push-failure>{push.failure ? t(push.failure) : t('app.pushes.failed_note')}</span>
            {:else if push.failure}
              <span class="small warned" data-push-failure>{t(push.failure)}</span>
            {/if}
          {:else}
            <span class="quiet">
              {push.pushed_at
                ? t('app.pushes.written_at', { when: formatDateTime(push.pushed_at, messages.locale) })
                : t('app.pushes.not_yet_written')}
            </span>
          {/if}
        {/snippet}
        {#snippet verb(push: CalendarPush)}
          {#if push.state === 'FAILED' && offered.includes(push.provider)}
            <Button size="sm" tone="secondary" isBusy={leaving === push.id} busyLabel={t('app.pushes.going')} onclick={() => void connectAgain(push)}>
              {t('app.pushes.connect_again')}
            </Button>
          {/if}
        {/snippet}
        {#snippet empty()}
          {#if pushes.length === 0}
            <EmptyState kind="unused" icon="calendar" title={t('app.pushes.none')} description={t('app.pushes.empty_body')} />
          {:else}
            <EmptyState kind="filtered" title={t('app.pushes.filtered_title')}>
              {#snippet action()}<Button onclick={() => (search = '')}>{t('app.page.clear_search')}</Button>{/snippet}
            </EmptyState>
          {/if}
        {/snippet}
      </DataTable>
    {/if}
  </ListPage>

  <FinalActDialog
    isOpen={asking !== undefined}
    title={t('app.pushes.disconnect_title', { calendar: asking ? providerName(asking) : '' })}
    body={t('app.pushes.disconnect_body')}
    confirmLabel={t('app.pushes.disconnect')}
    busyLabel={t('app.pushes.disconnecting')}
    onConfirm={async () => {
      if (!asking) return;
      try {
        await calendarPushes.disconnect(asking);
      } catch (error) {
        failure = renderProblem(error as never, messages).message;
      }
    }}
    onClose={() => (asking = undefined)}
  />
{/if}

{#snippet pushesFigure()}<span class="figure" data-pushes-figure>{figureText}</span>{/snippet}

<style>
  .what { display: block; font-weight: var(--fw-medium); }

  .figure { color: var(--text-secondary); font-variant-numeric: tabular-nums; }

  .mono { display: block; font-family: var(--font-mono); overflow-wrap: anywhere; }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .failed, .warned { display: block; margin-block-start: var(--sp-050); }
  .failed { color: var(--text-danger); }
  .warned { color: var(--text-warning); }

  .done { margin: 0; color: var(--text-success); }
</style>
