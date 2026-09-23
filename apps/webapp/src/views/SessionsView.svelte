<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where this reader is signed in.
  //
  // **A table, and the newest first** (ADR-0065 decision 3). It was a list of rows, each carrying
  // four facts as a sentence — which is readable at three sessions and unreadable at thirty, and
  // thirty is what a year of phones, laptops and browsers looks like. A column per fact is scanned
  // down rather than read across, and the row somebody is looking for is the one that appeared
  // while they were not looking, so it is at the top.
  //
  // **Ending the current session is a sign-out and is treated as one**: the list would otherwise
  // be re-read with a credential that has just stopped working, and the reader would meet a
  // refusal instead of the sign-in screen.

  import { untrack } from 'svelte';

  import { Button, EmptyState, ErrorState, Skeleton, Stack, Table } from '@hubtask/design-system/components';

  import SettingsHead from '../lib/frame/SettingsHead.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { sessions } from '../lib/data/sessions.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { session } from '../lib/session.svelte.ts';

  const account = $derived(actor.account);

  // The sessions are the account's own and take no parameter, so the read starts with the screen
  // rather than with an identifier arriving.
  $effect(() => untrack(() => sessions.open()));

  const held = $derived(sessions.state);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  // `$derived`, because the headings are words: a language chosen on the screen beside this one
  // changes them, and a list built once would keep the language it was built in.
  const columns = $derived([
    { id: 'client', label: t('app.sessions.client') },
    { id: 'created', label: t('app.sessions.created') },
    { id: 'used', label: t('app.sessions.last_used') },
    { id: 'network', label: t('app.sessions.network') },
    { id: 'end', label: t('app.sessions.end'), isLabelHidden: true, align: 'end' as const },
  ]);

  /**
   * The newest first, and this device first of all.
   *
   * The server answers them in its own order; which one a reader looks for is the one they do not
   * recognise, and that is the one that arrived last.
   */
  const shown = $derived(
    [...sessions.all].sort((a, b) => {
      if (a.current !== b.current) return a.current ? -1 : 1;
      return String(b.created_at).localeCompare(String(a.created_at));
    }),
  );

  /** An instant as this reader reads one: their locale, their clock (`i18n-l10n.md` §4). */
  function when(at: string | null | undefined): string {
    return at ? formatDateTime(at, messages.locale) : t('app.sessions.never_used');
  }

  async function endOne(id: string, isCurrent: boolean): Promise<void> {
    failure = undefined;
    try {
      await sessions.end(id);
      if (isCurrent) await session.signOut();
      else announcer.say(t('app.sessions.ended_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages);
    }
  }

  /** Ends every session, this one last by consequence rather than by order. */
  async function endEverywhere(): Promise<void> {
    failure = undefined;
    try {
      await sessions.endAll();
      announcer.say(t('app.sessions.ended_all_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages);
    }
    // Whatever the server managed, this tab is holding a credential it has been told to stop
    // trusting. Discarding it is not conditional on the call having succeeded.
    await session.signOut();
  }
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <Stack gap="300">
    <SettingsHead row="sessions" />

    <Stack gap="150">
      <p class="quiet">{t('app.sessions.intro')}</p>

      {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}

      {#if held === undefined || held.status === 'loading' || held.status === 'idle'}
        <div aria-busy="true"><Skeleton lines={2} /></div>
      {:else if held.status === 'failed'}
        <ErrorState
          title={renderProblem(held.error, messages).message}
          retryLabel={t('app.retry')}
          onRetry={() => sessions.open()}
        />
      {:else if shown.length === 0}
        <p class="quiet">{t('app.sessions.none')}</p>
      {:else}
        <Table label={t('app.sessions.title')} isLabelHidden {columns}>
          {#each shown as row (row.id)}
            <tr>
              <th scope="row" class="what">
                {row.user_agent || t('app.sessions.unknown_client')}
                {#if row.current}<span class="here">{t('app.sessions.this_device')}</span>{/if}
              </th>
              <td>{when(row.created_at)}</td>
              <td>{when(row.last_used_at)}</td>
              <td>{row.ip_class ?? '—'}</td>
              <td class="end">
                <Button tone="danger" size="sm" onclick={() => void endOne(row.id, row.current)}>
                  {row.current ? t('app.sessions.end_this') : t('app.sessions.end')}
                </Button>
              </td>
            </tr>
          {/each}
        </Table>

        <div class="everywhere">
          <!-- Said before it is pressed, because it ends this session too: a control whose
               consequence is "you are about to be signed out" has to say so where the finger is. -->
          <p class="quiet">{t('app.sign_out.everywhere_warning')}</p>
          <div>
            <Button tone="danger" onclick={() => void endEverywhere()}>{t('app.sign_out.everywhere')}</Button>
          </div>
        </div>
      {/if}
    </Stack>
  </Stack>
{/if}

<style>
  .what { font-weight: var(--fw-medium); }

  /* Which row is this device, beside its name rather than as a column of its own: it is true of
     exactly one row, and a column would be empty in every other. */
  .here {
    margin-inline-start: var(--sp-100);
    padding: var(--sp-025) var(--sp-100);
    border-radius: var(--r-full);
    background: var(--bg-surface-pressed);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-regular);
  }

  .end { text-align: end; }

  .everywhere { display: flex; flex-direction: column; gap: var(--sp-100); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .failure { margin: 0; color: var(--text-danger); }
</style>
