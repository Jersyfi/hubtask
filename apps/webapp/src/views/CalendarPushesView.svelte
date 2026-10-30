<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The calendar feeds the reader writes into their own calendar at Microsoft 365 or Google
  // (UC-INT-16), and the page the provider sends the browser back to after the sign-in.
  //
  // A push is the reader's own grant to their own calendar, so it is profile configuration like a
  // token: nobody else lists it. Connecting starts from a view's calendar subscriptions, where the
  // feed is; this screen says what is connected, whether it goes through, and takes one off.

  import { untrack } from 'svelte';

  import { Badge, Button, EmptyState, Stack, Table } from '@hubtask/design-system/components';
  import type { CalendarPush, CalendarPushList } from '@hubtask/sync-engine';

  import FinalActDialog from '../lib/frame/FinalActDialog.svelte';
  import SettingsHead from '../lib/frame/SettingsHead.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { calendarPushesPath, providerAnswerOf } from '../lib/data/calendarpush.ts';
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

  let failure = $state<string | undefined>(undefined);
  let connected = $state<CalendarPush | undefined>(undefined);
  let asking = $state<CalendarPush | undefined>(undefined);

  $effect(() => {
    const answer = untrack(() => (providerAnswer ? providerAnswerOf(providerAnswer) : undefined));
    if (!answer) return;
    void calendarPushes
      .complete(answer)
      .then((push) => (connected = push))
      .catch((error) => (failure = renderProblem(error as never, messages).message))
      .finally(() => onanswered?.());
  });

  const columns = $derived([
    { id: 'calendar', label: t('app.pushes.calendar_column') },
    { id: 'state', label: t('app.pushes.state_column') },
    { id: 'run', label: t('app.pushes.run_column') },
    { id: 'stop', label: t('app.pushes.disconnect'), isLabelHidden: true },
  ]);

  const providerName = (push: CalendarPush) => t(push.provider === 'GOOGLE' ? 'app.pushes.google' : 'app.pushes.microsoft');
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <Stack gap="300">
    <SettingsHead row="calendars" />

    <Stack gap="150">
      <p class="quiet">{t('app.pushes.intro')}</p>
      {#if connected}<p class="done" role="status">{t('app.pushes.connected', { calendar: providerName(connected) })}</p>{/if}
      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

      {#if pushes.length === 0}
        <p class="quiet">{t('app.pushes.none')}</p>
      {:else}
        <Table label={t('app.pushes.title')} isLabelHidden {columns}>
          {#each pushes as push (push.id)}
            <tr data-push={push.provider}>
              <th scope="row" class="what">
                {providerName(push)}
                {#if push.address}<span class="meta">{push.address}</span>{/if}
              </th>
              <td>
                {#if push.state === 'ACTIVE'}
                  <Badge tone="success">{t('app.pushes.state_active')}</Badge>
                {:else}
                  <Badge tone="warning">{t('app.pushes.state_failed')}</Badge>
                {/if}
                {#if push.failure}<span class="meta">{t(push.failure)}</span>{/if}
              </td>
              <td class="meta">
                {push.pushed_at ? formatDateTime(push.pushed_at, messages.locale) : t('app.pushes.not_yet')}
              </td>
              <td>
                <Button size="sm" tone="danger" onclick={() => (asking = push)}>{t('app.pushes.disconnect')}</Button>
              </td>
            </tr>
          {/each}
        </Table>
      {/if}
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

  .meta { display: block; color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-regular); }

  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; }

  .small { font-size: var(--fs-075); }

  .done { margin: 0; color: var(--text-success); }

  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
