<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What this workspace tells the reader about, and how.
  //
  // **A table, because it is one** (ADR-0065 decision 3). It was a list of rows, each drawing its
  // two switches wherever its own text ended: no two rows agreed, and a long category name — *I am
  // invited to the workspace* — pushed its switches out of line with every other row. One column
  // per question puts every switch of every row in the same place, and a reader compares a column
  // rather than reading each row to the end.
  //
  // **Nothing is compiled in.** The categories and the channels are the manifest's; a category
  // this version has no phrase for still renders, because `t` humanises an unknown code. The
  // **list** is the manifest's and only the wording is the catalogue's.

  import { untrack } from 'svelte';

  import { Checkbox, EmptyState, ErrorState, Skeleton, Stack, Table } from '@hubtask/design-system/components';

  import SettingsHead from '../lib/frame/SettingsHead.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { preferences } from '../lib/data/preferences.svelte.ts';
  import { categoriesOf, channelsOf, isAlwaysOn, preferenceFor } from '../lib/data/preferences.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  const account = $derived(actor.account);
  const accountId = $derived(account?.id);

  $effect(() => {
    const wanted = accountId;
    if (!wanted) return;
    return untrack(() => preferences.open(wanted));
  });

  const categories = $derived(categoriesOf(manifest.value));
  const channels = $derived(channelsOf(manifest.value));
  const rows = $derived(accountId ? preferences.of(accountId) : []);
  const reading = $derived(accountId ? preferences.stateOf(accountId) : undefined);

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  /**
   * The columns: what happens, then two per channel.
   *
   * The channel's own word joins the heading only where there is more than one, because a table
   * whose every heading repeats "By email" spends a third of its width saying one thing seven
   * times. With one channel the two questions are the two headings.
   */
  const columns = $derived([
    { id: 'category', label: t('app.profile.category_column') },
    ...channels.flatMap((channel) => [
      { id: `${channel}-enabled`, label: channels.length > 1 ? `${t(`app.profile.channel_${channel}`)} · ${t('app.profile.enabled')}` : t('app.profile.enabled') },
      { id: `${channel}-title`, label: channels.length > 1 ? `${t(`app.profile.channel_${channel}`)} · ${t('app.profile.include_title')}` : t('app.profile.include_title') },
    ]),
  ]);

  async function setRow(category: string, channel: string, next: { enabled?: boolean; include_title?: boolean }) {
    if (!accountId) return;
    const current = preferenceFor(rows, category, channel);
    failure = undefined;
    try {
      // Both switches travel: a row is a statement about a category rather than two values that
      // could drift apart.
      await preferences.setNotification(accountId, category, channel, {
        enabled: next.enabled ?? current?.enabled ?? true,
        include_title: next.include_title ?? current?.include_title ?? false,
      });
      announcer.say(t('app.profile.notification_saved_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages);
    }
  }
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <Stack gap="300">
    <SettingsHead row="notifications" />

    <Stack gap="150">
      <p class="quiet">{t('app.profile.notifications_hint')}</p>

      {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}

      {#if reading === undefined || reading.status === 'loading' || reading.status === 'idle'}
        <div aria-busy="true"><Skeleton lines={3} /></div>
      {:else if reading.status === 'failed'}
        <ErrorState
          title={renderProblem(reading.error, messages).message}
          retryLabel={t('app.retry')}
          onRetry={() => accountId && preferences.open(accountId)()}
        />
      {:else if categories.length === 0}
        <p class="quiet">{t('app.profile.none')}</p>
      {:else}
        <Table label={t('app.profile.notifications')} isLabelHidden {columns}>
          {#each categories as category (category)}
            <tr>
              <!-- A phrase per category the catalogue knows, and `humanise` for one it does not:
                   an installation that tells people about something newer still reads readably
                   rather than showing a key. -->
              <th scope="row" class="what">{t(`app.profile.category_${category}`)}</th>
              {#each channels as channel (channel)}
                {@const row = preferenceFor(rows, category, channel)}
                {@const alwaysOn = isAlwaysOn(category)}
                <td>
                  <Checkbox
                    label={t('app.profile.enabled')}
                    isLabelHidden
                    checked={row?.enabled ?? true}
                    disabledReason={alwaysOn ? t('app.profile.always_on') : undefined}
                    onchange={(event: Event) =>
                      void setRow(category, channel, { enabled: (event.currentTarget as HTMLInputElement).checked })}
                  />
                </td>
                <td>
                  <Checkbox
                    label={t('app.profile.include_title')}
                    isLabelHidden
                    checked={row?.include_title ?? false}
                    onchange={(event: Event) =>
                      void setRow(category, channel, { include_title: (event.currentTarget as HTMLInputElement).checked })}
                  />
                </td>
              {/each}
            </tr>
          {/each}
        </Table>
        <p class="quiet small">{t('app.profile.include_title_hint')}</p>
      {/if}
    </Stack>
  </Stack>
{/if}

<style>
  /* The category is the column that grows, so the two questions stand together at the end of the
     row rather than being pushed to opposite edges by a table sharing its width evenly. */
  .what { inline-size: 100%; font-weight: var(--fw-regular); }

  /* The one row nobody can switch off says why under its switch, and the column that carries that
     sentence keeps room for it - a reason wrapped one word to a line is a row four times as tall
     as its neighbours. Odd cells, because every channel draws "tell me" before "name the entry". */
  td:nth-of-type(odd) { min-inline-size: 24ch; }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .failure { margin: 0; color: var(--text-danger); }
</style>
