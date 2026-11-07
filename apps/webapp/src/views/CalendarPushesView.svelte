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

  import { Badge, Banner, Button, EmptyState, Stack } from '@hubtask/design-system/components';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import type { CalendarPush, CalendarPushList } from '@hubtask/sync-engine';

  import FinalActDialog from '../lib/frame/FinalActDialog.svelte';
  import SettingsHead from '../lib/frame/SettingsHead.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { calendarPushesPath, providerAnswerOf, pushFigure } from '../lib/data/calendarpush.ts';
  import { calendarPushes } from '../lib/data/calendarpush.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
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
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <Stack gap="300">
    <SettingsHead row="calendars">
      {#snippet figure()}{#if figureText}<span class="figure" data-pushes-figure>{figureText}</span>{/if}{/snippet}
    </SettingsHead>

    <Stack gap="200">
      <p class="quiet">{t('app.pushes.intro')}</p>
      {#if connected}<p class="done" role="status">{t('app.pushes.connected', { calendar: providerName(connected) })}</p>{/if}
      {#if failure}<FailureNote problem={failure} />{/if}

      {#each pushes as push (push.id)}
        <section class="panel" data-push={push.provider} aria-label={providerName(push)}>
          <Stack gap="100">
            <div class="row">
              <span class="what">{providerName(push)}</span>
              {#if push.state === 'ACTIVE'}
                <Badge tone="success">{t('app.pushes.state_active')}</Badge>
              {:else}
                <Badge tone="danger">{t('app.pushes.state_failed')}</Badge>
              {/if}
            </div>
            {#if push.address}<p class="quiet small mono">{push.address}</p>{/if}
            <p class="quiet small">
              {push.pushed_at
                ? t('app.pushes.written_at', { when: formatDateTime(push.pushed_at, messages.locale) })
                : t('app.pushes.not_yet_written')}
            </p>
            {#if push.state === 'FAILED'}
              <!-- The failure beside the act that fixes it: whoever presses it should know why. -->
              <Banner tone="danger">
                <Stack gap="100">
                  <span data-push-failure>{push.failure ? t(push.failure) : t('app.pushes.failed_note')}</span>
                  {#if offered.includes(push.provider)}
                    <div>
                      <Button size="sm" tone="primary" isBusy={leaving === push.id} busyLabel={t('app.pushes.going')} onclick={() => void connectAgain(push)}>
                        {t('app.pushes.connect_again')}
                      </Button>
                    </div>
                  {/if}
                </Stack>
              </Banner>
            {:else if push.failure}
              <!-- A run that did not go through while the push still writes: the next one tries again. -->
              <Banner tone="warning"><span data-push-failure>{t(push.failure)}</span></Banner>
            {/if}
            <div class="row">
              <Button size="sm" tone="danger" onclick={() => (asking = push)}>{t('app.pushes.disconnect')}</Button>
            </div>
          </Stack>
        </section>
      {:else}
        {#if read.state.status === 'ready'}
          <EmptyState kind="unused" icon="calendar" title={t('app.pushes.none')} description={t('app.pushes.empty_body')} />
        {/if}
      {/each}
      <p class="quiet small">{t('app.pushes.how')}</p>
    </Stack>
  </Stack>

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

<style>
  .what { font-weight: var(--fw-medium); }

  .figure { color: var(--text-secondary); font-variant-numeric: tabular-nums; }

  .row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .mono { font-family: var(--font-mono); overflow-wrap: anywhere; }

  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; }

  .small { font-size: var(--fs-075); }

  .done { margin: 0; color: var(--text-success); }
</style>
