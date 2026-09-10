<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The inbox around `JumbleInboxItem` (G-10, `automation.md` §4).
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
  // The control is offered and the server refuses it where the reader lacks `AUTOMATION`; a screen
  // that hid it would be a screen guessing at a permission, and this client does not do that.
  //
  // **No AI** (decision 11): `:suggest` exists and nothing here calls it.

  import { untrack } from 'svelte';

  import { Banner, Button, Input, JumbleInboxItem, OneTimeSecret, Select, Spinner, Stack, Textarea } from '@hubtask/design-system/components';

  import { containers } from '../lib/data/containers.svelte.ts';
  import { jumble, type IntakeToken, type JumbleEntry } from '../lib/data/jumble.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  let filter = $state('NEW');
  let converting = $state<string | undefined>(undefined);
  let destination = $state('');
  let title = $state('');
  let capturedSubject = $state('');
  let capturedBody = $state('');
  let minted = $state<IntakeToken | undefined>(undefined);
  let rotating = $state(false);
  let failure = $state<string | undefined>(undefined);
  let isWorking = $state(false);

  $effect(() => untrack(() => jumble.open(filter)));
  // The hubs, because a destination is a collection and a collection lives under one.
  $effect(() => untrack(() => containers.start()));
  $effect(() => {
    for (const hub of containers.hubs) untrack(() => containers.openLevel(hub.id));
  });

  // The address dies with the screen, like every other one-time value in this client.
  $effect(() => () => (minted = undefined));

  const reading = $derived(jumble.stateOf(filter));
  const entries = $derived(jumble.of(filter));

  /** Every collection under every hub: where an arrival may become work. */
  const destinations = $derived(
    containers.hubs.flatMap((hub) =>
      containers.collectionsOf(hub.id).map((collection) => ({
        value: collection.id,
        label: `${hub.name} · ${collection.name}`,
      })),
    ),
  );

  const filters = [
    { value: 'NEW', label: t('app.jumble.filter_new') },
    { value: 'PROCESSED', label: t('app.jumble.filter_processed') },
    { value: 'DISMISSED', label: t('app.jumble.filter_dismissed') },
  ];

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

  async function attempt(work: () => Promise<unknown>): Promise<void> {
    failure = undefined;
    isWorking = true;
    try {
      await work();
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
      destination = '';
      title = '';
      // Straight to what it produced. The link is the point of the provenance pair, and a reader
      // who has just converted something wants to see it rather than find it.
      if (converted.target_item_id) onnavigate(`/items/${converted.target_item_id}`);
    });
  }

  async function capture(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!capturedSubject.trim() && !capturedBody.trim()) return;
    await attempt(async () => {
      await jumble.capture(capturedSubject.trim(), capturedBody.trim());
      capturedSubject = '';
      capturedBody = '';
    });
  }
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.jumble.title')}</h1>
    <p class="quiet">{t('app.jumble.intro')}</p>

    {#if failure}<Banner tone="danger">{failure}</Banner>{/if}

    <Select label={t('app.jumble.showing')} bind:value={filter} options={filters} />

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
            {#snippet actions()}
              {#if entry.status === 'NEW'}
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
                  <Button
                    tone="primary"
                    onclick={() => {
                      converting = entry.id;
                      destination = '';
                      title = subjectOf(entry);
                    }}
                  >
                    {t('app.jumble.convert')}
                  </Button>
                  <Button
                    tone="subtle"
                    isBusy={isWorking}
                    busyLabel={t('app.jumble.working')}
                    onclick={() => void attempt(() => jumble.dismiss(entry.id))}
                  >
                    {t('app.jumble.dismiss')}
                  </Button>
                {/if}
              {/if}
            {/snippet}
          </JumbleInboxItem>
        {:else}
          <p class="quiet">{t('app.jumble.nothing_here')}</p>
        {/each}

        {#if jumble.moreAfter(filter)}
          <!-- Cursor pagination, never page numbers: the API has none, so nothing here implies one. -->
          <div>
            <Button tone="secondary" onclick={() => void jumble.more(filter)}>
              {t('app.jumble.more')}
            </Button>
          </div>
        {/if}
      </Stack>
    {/if}

    <Stack gap="150">
      <h2 class="section">{t('app.jumble.capture_title')}</h2>
      <p class="quiet small">{t('app.jumble.capture_hint')}</p>
      <form onsubmit={capture}>
        <Stack gap="150">
          <Input label={t('app.jumble.capture_subject')} bind:value={capturedSubject} />
          <Textarea label={t('app.jumble.capture_body')} bind:value={capturedBody} rows={3} />
          <div>
            <Button type="submit" tone="primary" isBusy={isWorking} busyLabel={t('app.jumble.capturing')}>
              {t('app.jumble.capture')}
            </Button>
          </div>
        </Stack>
      </form>
    </Stack>

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
              })}
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

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  form { margin: 0; }
</style>
