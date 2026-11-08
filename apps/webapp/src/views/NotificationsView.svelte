<!-- SPDX-License-Identifier: Apache-2.0
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

  import { EmptyState, SettingsLayout, SettingsSection, Skeleton } from '@hubtask/design-system/components';
  import FailureState from '../lib/frame/FailureState.svelte';

  import { page } from '../lib/frame/page.svelte.ts';
  import { profileHead, profileTitle } from '../lib/profile/profilehead.ts';
  import NotificationRow from '../lib/profile/NotificationRow.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { preferences } from '../lib/data/preferences.svelte.ts';
  import { categoriesOf, channelsOf, isAlwaysOn, preferenceFor } from '../lib/data/preferences.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  const { onnavigate }: { onnavigate?: (path: string) => void } = $props();

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
   * The boxes being written, as `category:channel:field`: each shows its progress in its own place
   * and confirms as a passing message (design-system.md §4, case 7).
   */
  let writing = $state<ReadonlySet<string>>(new Set());
  const busyOf = (category: string) =>
    new Set([...writing].filter((key) => key.startsWith(`${category}:`)).map((key) => key.slice(category.length + 1)));

  async function setRow(category: string, channel: string, next: { enabled?: boolean; include_title?: boolean }) {
    if (!accountId) return;
    const current = preferenceFor(rows, category, channel);
    const key = `${category}:${channel}:${next.enabled !== undefined ? 'enabled' : 'include_title'}`;
    writing = new Set([...writing, key]);
    failure = undefined;
    try {
      // Both switches travel: a row is a statement about a category rather than two values that
      // could drift apart.
      await preferences.setNotification(accountId, category, channel, {
        enabled: next.enabled ?? current?.enabled ?? true,
        include_title: next.include_title ?? current?.include_title ?? false,
      });
      toast.show({ text: t('app.profile.notification_saved_announced'), tone: 'success' });
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      writing = new Set([...writing].filter((each) => each !== key));
    }
  }

  const title = $derived(profileTitle('notifications'));
  $effect(() => page.entitle(title));

  /** A managed account (ADR-0074 §5): no address, so no mail and no table of switches for it. */
  const withoutMailbox = $derived(account?.kind === 'USER' && !account.email);
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <!-- A settings page of one section (page-system §9.5): every box is written the moment it is
       pressed, so the section has no save bar (PS-10). -->
  <SettingsLayout
    head={profileHead('notifications')}
    nav={{ label: t('app.settings_page.sections', { name: title }), sections: [{ id: 'notifications', label: title, href: '/profile/notifications' }], current: 'notifications' }}
    view="detail"
    back={{ label: t('app.nav.profile'), onback: () => onnavigate?.('/profile') }}
  >
    {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}

    <!-- The lead promises rows, so it stands only where the rows can be drawn. -->
    <SettingsSection title={t('app.profile.notifications_section')} description={withoutMailbox ? undefined : t('app.profile.notifications_hint')}>
      {#if withoutMailbox}
        <!-- A managed account (ADR-0074 §5): mail has nowhere to go, and switches for it would be
             a promise nothing keeps. -->
        <p class="quiet pad">{t('app.profile.no_mailbox')}</p>
      {:else if reading === undefined || reading.status === 'loading' || reading.status === 'idle'}
        <div class="pad" aria-busy="true"><Skeleton lines={3} /></div>
      {:else if reading.status === 'failed'}
        <FailureState problem={renderProblem(reading.error, messages)} onRetry={() => accountId && preferences.open(accountId)()} />
      {:else if categories.length === 0}
        <EmptyState kind="unused" title={t('app.profile.none')} />
      {:else}
        {#each categories as category (category)}
          <NotificationRow
            {category}
            {channels}
            {rows}
            isAlwaysOn={isAlwaysOn(category)}
            busy={busyOf(category)}
            onset={(channel, next) => void setRow(category, channel, next)}
          />
        {/each}
      {/if}
    </SettingsSection>
    {#if !withoutMailbox && categories.length > 0}<p class="quiet small">{t('app.profile.include_title_hint')}</p>{/if}
  </SettingsLayout>
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  .pad { padding: var(--sp-200) var(--sp-250); }

  .small { font-size: var(--fs-075); }

  .failure { margin: 0; color: var(--text-danger); }
</style>
