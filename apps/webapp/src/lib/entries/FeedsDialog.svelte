<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Calendar subscriptions: making one, seeing which exist, and stopping one.
  //
  // **The address is a credential and is shown once.** "Whoever has the URL reads the view as its
  // owner does" — so it is said in those words, before the address rather than after it. It is held
  // in one `$state` for as long as this dialog is open and in nothing else: not in the store, not
  // in a URL this client composes, not in a log. Closing the dialog is what forgets it, and that is
  // the whole design rather than an oversight.
  //
  // **The list carries no token at all**, because the API does not answer one: what is stored is a
  // hash. A feed whose address was lost is revoked and made again, which is what the sentence says.
  //
  // **Revoking names the consequence.** Every calendar subscribed to that address stops, at once,
  // without being told — so the confirmation says that rather than asking "are you sure".

  import { Badge, Button, Dialog, Inline, Input, Stack } from '@hubtask/design-system/components';
  import type { CalendarFeed, SavedView } from '@hubtask/sync-engine';

  import { feeds, views } from '../data/views.svelte.ts';
  import { feedStateOf } from '../data/views.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    /** The view a new subscription would serve, when this was opened from one. */
    view?: SavedView;
    /** The collection whose views name the feeds in the list. */
    collectionId: string;
  }

  let { isOpen = $bindable(), view, collectionId }: Props = $props();

  const held = $derived(feeds.all);
  const known = $derived(views.of(collectionId));

  /**
   * The address, for as long as this dialog is open.
   *
   * Deliberately a local `$state` and deliberately not in the store: "shown once" is only true if
   * there is one place it exists, and this is it.
   */
  let secret = $state<string | undefined>(undefined);
  let copied = $state(false);
  let revoking = $state<CalendarFeed | undefined>(undefined);
  let isWorking = $state(false);
  let failure = $state<string | undefined>(undefined);

  $effect(() => {
    if (isOpen) return;
    // Closing forgets it. Nothing else does, which is why this is here rather than in a handler.
    secret = undefined;
    copied = false;
  });

  function nameOf(feed: CalendarFeed): string {
    if (!feed.view_id) return t('app.feeds.unknown_view');
    return known.find((each) => each.id === feed.view_id)?.name ?? t('app.feeds.unknown_view');
  }

  async function attempt(work: () => Promise<unknown>): Promise<void> {
    isWorking = true;
    failure = undefined;
    try {
      await work();
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWorking = false;
    }
  }

  function mint() {
    if (!view) return;
    void attempt(async () => {
      const made = await feeds.create({ view_id: view.id }, crypto.randomUUID());
      secret = made.url;
      copied = false;
    });
  }

  function copy() {
    if (!secret) return;
    void navigator.clipboard
      .writeText(secret)
      .then(() => (copied = true))
      .catch(() => {
        // A clipboard a browser refuses is not a failure worth a sentence: the address is on
        // screen and selectable, which is the fallback every copy control has.
        copied = false;
      });
  }

  function confirmRevoke() {
    const target = revoking;
    if (!target) return;
    void attempt(async () => {
      await feeds.revoke(target.id);
      revoking = undefined;
    });
  }
</script>

<Dialog bind:isOpen title={t('app.feeds.title')} dismissLabel={t('app.workspace.cancel')}>
  <Stack gap="200">
    {#if secret}
      <Stack gap="150">
        <!-- The sentence comes before the address, not after it. -->
        <p class="warning">{t('app.feeds.url_once')}</p>
        <Input label={t('app.feeds.url')} value={secret} readonly />
        <Inline gap="100">
          <Button tone="secondary" onclick={copy}>{t('app.feeds.copy')}</Button>
          {#if copied}<span class="quiet">{t('app.feeds.copied')}</span>{/if}
          <Button onclick={() => (secret = undefined)}>{t('app.feeds.done')}</Button>
        </Inline>
      </Stack>
    {:else}
      {#if view}
        <div>
          <Button isBusy={isWorking} busyLabel={t('app.workspace.saving')} onclick={mint}>
            {t('app.feeds.create')}
          </Button>
        </div>
      {/if}

      {#if held.length === 0}
        <p class="quiet">{t('app.feeds.none')}</p>
      {:else}
        <ul class="list">
          {#each held as feed (feed.id)}
            {@const state = feedStateOf(feed)}
            <li>
              <div class="row">
                <div>
                  <span class="name">{t('app.feeds.for_view', { name: nameOf(feed) })}</span>
                  <span class="meta">
                    {t('app.feeds.made', { when: formatDateTime(feed.created_at, messages.locale) })}
                  </span>
                </div>
                <Inline gap="050">
                  {#if state === 'revoked'}
                    <Badge>{t('app.feeds.state_revoked')}</Badge>
                  {:else if state === 'orphaned'}
                    <!-- A feed is the subscriber's and a view is the workspace's, so a deleted view
                         leaves a feed that serves nothing — and says which of the two it is. -->
                    <Badge tone="warning">{t('app.feeds.state_orphaned')}</Badge>
                  {:else}
                    <Badge tone="success">{t('app.feeds.state_active')}</Badge>
                  {/if}
                  {#if state !== 'revoked'}
                    <Button size="sm" tone="secondary" onclick={() => (revoking = feed)}>
                      {t('app.feeds.revoke')}
                    </Button>
                  {/if}
                </Inline>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    {/if}

    {#if failure}<p class="failure">{failure}</p>{/if}
  </Stack>
</Dialog>

{#if revoking}
  <Dialog
    isOpen={true}
    title={t('app.feeds.revoke_title')}
    dismissLabel={t('app.workspace.cancel')}
    onClose={() => (revoking = undefined)}
  >
    <Stack gap="150">
      <p class="quiet">{t('app.feeds.revoke_explains')}</p>
      {#if failure}<p class="failure">{failure}</p>{/if}
      <Inline gap="100">
        <Button tone="danger" isBusy={isWorking} busyLabel={t('app.workspace.saving')} onclick={confirmRevoke}>
          {t('app.feeds.revoke')}
        </Button>
        <Button tone="secondary" onclick={() => (revoking = undefined)}>{t('app.workspace.cancel')}</Button>
      </Inline>
    </Stack>
  </Dialog>
{/if}

<style>
  .list { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-100); }

  .row { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }

  .name { display: block; font-weight: var(--fw-semibold); }

  .meta { display: block; color: var(--text-secondary); font-size: var(--fs-075); }

  .warning { margin: 0; color: var(--text-warning); max-width: 64ch; }

  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); max-width: 64ch; }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
