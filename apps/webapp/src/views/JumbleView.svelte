<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The inbox around `JumbleInboxItem` (`automation.md` §4).
  //
  // **An entry is decided about exactly once**, and a dismissal is not a deletion: the row stays
  // readable and ages out by retention rule. So nothing here removes anything from the list — the
  // filter is the reader's choice, not the screen quietly tidying up after them.
  //
  // **A conversion needs a destination**, and the destination is a collection the reader may write
  // in. What comes back is the entry with `target_item_id` on it, which is the other half of the
  // provenance pair — following that link is what makes the inbox trustworthy rather than a place
  // things disappear into.
  //
  // **The intake address is a credential in a URL**, which is why it is minted rather than shown.
  // Only for somebody holding `AUTOMATION` on the workspace, as the manifest answers it: pointing
  // outside input at the workspace is that power, and a control the server refuses is absent (P-05).
  //
  // **Each control for the permission it needs**, read from the same answer: the list for anybody
  // the screen is drawn for (`READ`), capturing and settling an arrival - convert, dismiss, ask -
  // with `WRITE_ITEMS`. A reader who may only read sees the list and nothing to press.
  //
  // **AI, where the manifest says there is one**: a control on each undecided card asks
  // `:suggest`, and the proposal renders in the card's slot as an `AISuggestion` whose acceptance
  // converts the entry with the proposed fields into the collection the person chooses. With AI
  // off the card is the plain card - the control and the slot are not rendered.

  import { untrack } from 'svelte';

  import {
    Banner,
    Button,
    Checkbox,
    Input,
    JumbleInboxItem,
    LoadMore,
    OneTimeSecret,
    PageHeader,
    Select,
    Spinner,
    Stack,
    ViewSwitcher,
  } from '@hubtask/design-system/components';

  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { jumble, type IntakeToken, type JumbleEntry } from '../lib/data/jumble.svelte.ts';
  import { suggestions } from '../lib/data/suggestions.svelte.ts';
  import { selection } from '../lib/data/selection.svelte.ts';
  import { askEach } from '../lib/data/askEach.ts';
  import { isPending } from '../lib/data/suggestions.ts';
  import JumbleProposal from '../lib/entries/JumbleProposal.svelte';
  import MailAddressField from '../lib/mail/MailAddressField.svelte';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem, renderRefusal } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  let filter = $state('NEW');
  let converting = $state<string | undefined>(undefined);
  let destination = $state('');
  let title = $state('');
  let minted = $state<IntakeToken | undefined>(undefined);
  let rotating = $state(false);
  let failure = $state<string | undefined>(undefined);
  let isWorking = $state(false);

  $effect(() => untrack(() => jumble.open(filter)));
  // Where the reader may turn an arrival into work, as the server answers it.
  $effect(() => {
    if (mayWrite) untrack(() => void jumble.loadDestinations());
  });

  // The address dies with the screen, like every other one-time value in this client.
  $effect(() => () => (minted = undefined));

  /** What the reader may do here, from what the manifest says they hold on the workspace. */
  const mayWrite = $derived(holdsHere('WRITE_ITEMS') === true);
  const mayRotate = $derived(holdsHere('AUTOMATION') === true);

  /** What the last press of *Show more* brought, said by `LoadMore`'s live region; cleared by a new filter. */
  let arrived = $state<string | undefined>(undefined);
  let isLoadingMore = $state(false);
  /** Why the last press of *Show more* brought nothing; cleared by the next press and a new filter. */
  let moreFailed = $state<string | undefined>(undefined);
  $effect(() => {
    void filter;
    arrived = undefined;
    moreFailed = undefined;
  });

  /** The next page, appended, and what arrived said once it has (P-13). */
  async function more(): Promise<void> {
    const current = filter;
    const before = jumble.of(current).length;
    moreFailed = undefined;
    isLoadingMore = true;
    try {
      await jumble.more(current);
      arrived = t('app.jumble.arrived', { count: jumble.of(current).length - before });
    } catch (error) {
      // Beside the button that asked, not in the page's banner: what is already shown is still right.
      moreFailed = renderProblem(error as never, messages).message;
    } finally {
      isLoadingMore = false;
    }
  }

  const reading = $derived(jumble.stateOf(filter));
  const entries = $derived(jumble.of(filter));

  // Absent when AI is off - not disabled with a reason (design-system.md §4, CapabilityGate).
  const hasAi = $derived(manifest.value?.features?.ai_suggestions === true);
  let askFailure = $state<string | undefined>(undefined);

  /** Straight to what the acceptance made, the way a conversion by hand goes. */
  async function accepted(entryId: string): Promise<void> {
    const became = await jumble.whatBecameOf(entryId);
    if (became?.target_item_id) onnavigate(`/items/${became.target_item_id}`);
  }

  async function ask(entry: JumbleEntry): Promise<void> {
    askFailure = undefined;
    try {
      await suggestions.askJumble(entry.id);
    } catch (error) {
      askFailure = renderProblem(error as never, messages).message;
    }
  }

  // **Asking about a selection** (UC-JUM-07 check 12): a mode, off until the page's menu starts
  // it (design-system.md §11.9), offered where AI is and the reader may write, over the undecided
  // entries on screen - the loaded pages, nothing beyond them. Each is asked on its own and
  // followed on its own card; a refusal that holds for all stops the rest and is said once.
  const selectable = $derived(hasAi && mayWrite && filter === 'NEW');
  const undecided = $derived(entries.filter((entry) => entry.status === 'NEW').map((entry) => entry.id));
  $effect(() => {
    const visible = undecided;
    untrack(() => selection.keepVisible(visible));
  });
  // A selection is about what is in front of somebody: a new filter, or leaving, ends it.
  $effect(() => {
    void filter;
    untrack(() => selection.clear());
  });
  $effect(() => () => selection.clear());
  let askingSelection = $state(false);
  let selectionFailure = $state<string | undefined>(undefined);

  async function askSelected(): Promise<void> {
    if (askingSelection || selection.count === 0) return;
    askingSelection = true;
    selectionFailure = undefined;
    const result = await askEach([...selection.ids], (id) => suggestions.queueJumble(id));
    askingSelection = false;
    if (result.stoppedBy !== undefined) {
      selectionFailure = renderProblem(result.stoppedBy as never, messages).message;
    } else if (result.failed.length > 0) {
      selectionFailure = t('app.jumble.selection_some_failed', { count: result.failed.length });
    } else {
      announcer.say(t('app.jumble.selection_asked'));
    }
    selection.stop();
  }

  function onkeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape' && selection.isOn) selection.stop();
  }

  const pageMenu = $derived(
    selectable
      ? {
          label: t('app.jumble.actions'),
          items: [{ id: 'select', label: t('app.bulk.select_mode'), icon: 'square-check' as const }],
          onselect: (id: string) => {
            if (id === 'select') selection.start();
          },
        }
      : undefined,
  );

  /** Where the reader may turn an arrival into work: only collections a conversion would accept (P-05). */
  const destinations = $derived(
    (jumble.destinations ?? []).map((collection) => ({
      value: collection.id,
      label: `${collection.hub_name} · ${collection.name}`,
    })),
  );

  const filters = $derived([
    { value: 'NEW', label: t('app.jumble.filter_new') },
    { value: 'PROCESSED', label: t('app.jumble.filter_processed') },
    { value: 'DISMISSED', label: t('app.jumble.filter_dismissed') },
  ]);

  const when = (at: string | null | undefined) =>
    at ? formatDateTime(at, messages.locale) : t('app.jumble.unknown_time');

  /** What this arrival is called, where it has nothing to be called. */
  const subjectOf = (entry: JumbleEntry) => entry.raw_subject?.trim() || t('app.jumble.no_subject');

  const statusWord = (status: string) =>
    messages.has(`app.jumble.status_${status.toLowerCase()}`)
      ? t(`app.jumble.status_${status.toLowerCase()}`)
      : status;

  const channelWord = (channel: string) =>
    messages.has(`app.jumble.channel_${channel.toLowerCase()}`)
      ? t(`app.jumble.channel_${channel.toLowerCase()}`)
      : channel;

  async function attempt(work: () => Promise<unknown>, said?: string): Promise<void> {
    failure = undefined;
    isWorking = true;
    try {
      await work();
      // Said out loud on success (4.1.3): what changed is on the screen, and a reader who cannot
      // see the screen is told the same thing once, through the one live region.
      if (said) announcer.say(said);
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWorking = false;
    }
  }

  async function convert(entry: JumbleEntry): Promise<void> {
    if (!destination) return;
    await attempt(async () => {
      const converted = await jumble.convert(entry.id, {
        collectionId: destination,
        title: title.trim() || undefined,
      });
      converting = undefined;
      // The destination is kept, not cleared: the next arrival goes to the same collection more
      // often than not, and asking for it again would be asking for what was just given (3.3.7).
      // The title is the arrival's own and is not kept.
      title = '';
      // Straight to what it produced. The link is the point of the provenance pair, and a reader
      // who has just converted something wants to see it rather than find it.
      if (converted.target_item_id) onnavigate(`/items/${converted.target_item_id}`);
    });
  }

  /** A collection's name from the destinations the reader may write into. */
  function placeName(id: string | null | undefined): string {
    const found = (jumble.destinations ?? []).find((collection) => collection.id === id);
    return found ? found.name : t('app.jumble.meant_for_item');
  }

  /** Sorts a mail into the place it was meant for: an entry there, or a comment on the entry. */
  async function sortIn(entry: JumbleEntry): Promise<void> {
    if (!entry.meant_for_id) return;
    await attempt(async () => {
      const settled =
        entry.meant_for_kind === 'ITEM'
          ? await jumble.comment(entry.id, entry.meant_for_id ?? undefined)
          : await jumble.convert(entry.id, { collectionId: entry.meant_for_id ?? '', title: subjectOf(entry) });
      if (settled.target_item_id) onnavigate(`/items/${settled.target_item_id}`);
    }, t('app.jumble.sorted_announced'));
  }

  // The bar carries the page's title on a phone (ADR-0061 decision 1's table); the head then
  // reads its heading rather than drawing it, so the screen keeps one heading.
  $effect(() => page.entitle(t('app.jumble.title')));
</script>

<svelte:window {onkeydown} />

<div class="screen">
  <Stack gap="300">
    <!-- `data-tour`: where the tour points for "where things arrive before they are work". -->
    <div data-tour="jumble">
      <PageHeader
        title={t('app.jumble.title')}
        isTitleInBar={viewport.isCompact}
        menu={pageMenu}
        isMenuInBar={viewport.isCompact}
        onmenu={(offered) => page.offer(offered)}
      />
      <p class="quiet">{t('app.jumble.intro')}</p>
      <!-- The jumble's own address, where people look for it (ADR-0092 M1). -->
      <MailAddressField target={{ kind: 'JUMBLE' }} lead={t('app.mail.jumble_lead')} mayShape={mayRotate} />
    </div>

    {#if failure}<Banner tone="danger">{failure}</Banner>{/if}
    {#if askFailure}<Banner tone="danger">{askFailure}</Banner>{/if}
    <!-- Said once for the page rather than on every card it held back: proposals prepared on
         arrival met a spent budget, and the arrivals wait for somebody to ask. -->
    {#if hasAi && suggestions.arrivalBudgetSpent}
      <p class="quiet" data-arrival-budget>{renderRefusal(suggestions.arrivalBudgetSpent, messages)}</p>
    {/if}

    <!-- Which arrivals: three states, one chosen - the segments the collection's layouts use, not a
         field the width of the page. -->
    <div class="states">
      <ViewSwitcher label={t('app.jumble.showing')} views={filters.map((each) => ({ id: each.value, label: each.label }))} selected={filter} onselect={(id) => (filter = id as typeof filter)} />
    </div>

    {#if selectable && selection.isOn}
      <div class="selection" role="toolbar" aria-label={t('app.jumble.actions')} data-jumble-selection>
        <span class="count">{t('app.bulk.selected', { count: selection.count })}</span>
        <Button tone="subtle" onclick={() => selection.allVisible()}>{t('app.bulk.select_all')}</Button>
        <Button
          tone="primary"
          icon="sparkles"
          data-ai
          isBusy={askingSelection}
          busyLabel={t('app.jumble.asking_selection')}
          disabledReason={selection.count === 0 ? t('app.jumble.select_first') : undefined}
          onclick={() => void askSelected()}
        >
          {t('app.jumble.ask_selection')}
        </Button>
        <Button tone="subtle" onclick={() => selection.stop()}>{t('app.bulk.done')}</Button>
      </div>
    {/if}
    {#if selectionFailure}<p class="quiet" role="alert">{selectionFailure}</p>{/if}

    {#if reading.status === 'loading' || reading.status === 'idle'}
      <p class="waiting"><Spinner label={t('app.jumble.reading')} /> <span>{t('app.jumble.reading')}</span></p>
    {:else}
      <Stack gap="150">
        {#each entries as entry (entry.id)}
          <JumbleInboxItem
            subject={subjectOf(entry)}
            excerpt={entry.raw_body ?? undefined}
            channelLabel={channelWord(entry.channel)}
            sender={entry.sender ?? undefined}
            senderLabel={t('app.jumble.sender')}
            receivedLabel={when(entry.received_at)}
            receivedAt={entry.received_at}
            status={entry.status}
            statusLabel={statusWord(entry.status)}
            targetHref={entry.target_item_id ? `/items/${entry.target_item_id}` : undefined}
            targetLabel={entry.target_item_id ? t('app.jumble.see_what_it_became') : undefined}
            dismissedNote={entry.status === 'DISMISSED' ? t('app.jumble.dismissed_note') : undefined}
          >
            {#snippet suggestion()}
              {#if hasAi && mayWrite && entry.status === 'NEW'}
                <JumbleProposal {entry} {destinations} chosen={destination} onaccepted={(id) => void accepted(id)} />
              {/if}
            {/snippet}
            {#snippet actions()}
              {#if selectable && selection.isOn && entry.status === 'NEW'}
                <Checkbox
                  label={t('app.bulk.select', { title: subjectOf(entry) })}
                  checked={selection.has(entry.id)}
                  onchange={() => selection.pick(undecided, entry.id)}
                />
              {:else if mayWrite && entry.status === 'NEW'}
                {#if entry.meant_for_id && converting !== entry.id}
                  <!-- Meant for a place, so sorting it in is one step (ADR-0092 decision 5). -->
                  <Button
                    tone="primary"
                    size="sm"
                    icon="arrow-right"
                    isBusy={isWorking}
                    busyLabel={t('app.jumble.converting')}
                    data-sort-in
                    onclick={() => void sortIn(entry)}
                  >
                    {entry.meant_for_kind === 'ITEM'
                      ? t('app.jumble.sort_as_comment')
                      : t('app.jumble.sort_into', { place: placeName(entry.meant_for_id) })}
                  </Button>
                {/if}
                {#if converting === entry.id}
                  <Stack gap="150">
                    <Select
                      label={t('app.jumble.destination')}
                      hint={t('app.jumble.destination_hint')}
                      bind:value={destination}
                      placeholder={t('app.jumble.choose_destination')}
                      options={destinations}
                    />
                    <Input
                      label={t('app.jumble.entry_title')}
                      hint={t('app.jumble.entry_title_hint')}
                      bind:value={title}
                    />
                    <div class="row">
                      <Button
                        tone="primary"
                        isBusy={isWorking}
                        busyLabel={t('app.jumble.converting')}
                        onclick={() => void convert(entry)}
                      >
                        {t('app.jumble.make_it')}
                      </Button>
                      <Button tone="subtle" onclick={() => (converting = undefined)}>
                        {t('app.jumble.cancel')}
                      </Button>
                    </div>
                  </Stack>
                {:else}
                  <!-- One arrival among many: its acts at the size a row's are, none of them the
                       page's one primary. -->
                  <Button
                    tone="secondary"
                    size="sm"
                    icon="arrow-right"
                    onclick={() => {
                      converting = entry.id;
                      title = subjectOf(entry);
                    }}
                  >
                    {t('app.jumble.convert')}
                  </Button>
                  <Button
                    tone="subtle"
                    size="sm"
                    isBusy={isWorking}
                    busyLabel={t('app.jumble.working')}
                    onclick={() => void attempt(() => jumble.dismiss(entry.id), t('app.jumble.dismissed_announced'))}
                  >
                    {t('app.jumble.dismiss')}
                  </Button>
                  <!-- Absent for an arrival with neither subject nor text: there is nothing to ask
                       about, and nothing the reader could change about that - the content came
                       from outside. Waiting for an answer is said at the button. -->
                  {#if hasAi && (entry.raw_subject?.trim() || entry.raw_body?.trim())}
                    <Button
                      tone="secondary"
                      size="sm"
                      icon="sparkles"
                      data-ai
                      disabledReason={isPending(suggestions.askingOf(entry.id)) ? t('app.suggestions.pending') : undefined}
                      onclick={() => void ask(entry)}
                    >
                      {t('app.jumble.ask')}
                    </Button>
                  {/if}
                {/if}
              {/if}
            {/snippet}
          </JumbleInboxItem>
        {:else}
          <p class="quiet">{t('app.jumble.nothing_here')}</p>
        {/each}

        <!-- Cursor pagination, never page numbers: the API has none, so nothing here implies one.
             Drawn whole while something is to be said after the last page: the live regions go
             with the component, and one removed with the button is one nobody hears. -->
        {#if jumble.moreAfter(filter) || arrived || moreFailed}
          <LoadMore
            label={t('app.jumble.more')}
            busyLabel={t('app.jumble.loading_more')}
            hasMore={Boolean(jumble.moreAfter(filter))}
            isBusy={isLoadingMore}
            arrivedLabel={arrived}
            failedLabel={moreFailed}
            onLoadMore={() => void more()}
          />
        {/if}
      </Stack>
    {/if}


    {#if mayRotate}
      <Stack gap="150" data-mail-section>
        <h2 class="section">{t('app.mail.section')}</h2>
        <MailAddressField target={{ kind: 'JUMBLE' }} shape="panel" mayShape />
      </Stack>
    {/if}

    {#if mayRotate}
      <Stack gap="150">
        <h2 class="section">{t('app.jumble.intake_title')}</h2>
        <p class="quiet small">{t('app.jumble.intake_hint')}</p>

        {#if minted}
          <OneTimeSecret
            value={minted.token}
            label={t('app.jumble.intake_address')}
            hint={t('app.jumble.intake_minted_hint')}
            revealLabel={t('app.jumble.reveal')}
            hideLabel={t('app.jumble.hide')}
            copyLabel={t('app.jumble.copy')}
            copiedLabel={t('app.jumble.copied')}
            acknowledgementLabel={t('app.jumble.kept')}
            notAcknowledgedReason={t('app.jumble.keep_first')}
            dismissLabel={t('app.jumble.done')}
            onDismiss={() => (minted = undefined)}
          />
        {:else if rotating}
          <!-- Said before the button: whatever posts to the current address stops at this moment. -->
          <Banner tone="warning">{t('app.jumble.rotate_cost')}</Banner>
          <div class="row">
            <Button
              tone="danger"
              isBusy={isWorking}
              busyLabel={t('app.jumble.rotating')}
              onclick={() =>
                void attempt(async () => {
                  minted = await jumble.rotateIntake();
                  rotating = false;
                }, t('app.jumble.rotated_announced'))}
            >
              {t('app.jumble.rotate_now')}
            </Button>
            <Button tone="subtle" onclick={() => (rotating = false)}>{t('app.jumble.cancel')}</Button>
          </div>
        {:else}
          <div>
            <Button tone="secondary" onclick={() => (rotating = true)}>{t('app.jumble.rotate')}</Button>
          </div>
        {/if}
      </Stack>
    {/if}
  </Stack>
</div>

<style>
  /* As wide as its segments, at the page's edge - not the width of the page. */
  .states { display: flex; }

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  .selection { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .count { font-weight: var(--fw-semibold); margin-inline-end: var(--sp-100); }
</style>
