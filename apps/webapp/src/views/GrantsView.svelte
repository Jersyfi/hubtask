<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The apps that may act in this workspace as the reader.
  //
  // The same question the sessions ask, asked about apps: what is currently able to act as me, and
  // how do I stop it. Withdrawing takes effect on the app's next request, which the confirmation
  // says rather than implying that what is running now stops mid-sentence.
  //
  // **Rows, not a table** (UC-SYN-07/3, ADR-0096): a mark, the name, when it was allowed and
  // last used, and the one action. The installed apps carry the app's icon, their system and device
  // and *Sign out*, because signing the app out is what withdrawing their grant does (ADR-0096).

  import { untrack } from 'svelte';

  import { Button, EmptyState, Logo, Stack } from '@hubtask/design-system/components';

  import FinalActDialog from '../lib/frame/FinalActDialog.svelte';
  import SettingsHead from '../lib/frame/SettingsHead.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { consent, type Grant } from '../lib/data/consent.svelte.ts';
  import { grantOrder, grantUse, initialsOf } from '../lib/data/grants.ts';
  import { formatLongDay, formatTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  const account = $derived(actor.account);

  $effect(() => untrack(() => consent.openGrants()));

  let failure = $state<string | undefined>(undefined);

  const rows = $derived(grantOrder(consent.grants));

  /** An installed app's grant is named by its system and device (ADR-0096). */
  function nameOf(grant: Grant): string {
    return grant.app_platform
      ? t('app.grants.app_name', { platform: t(`app.app_platform.${grant.app_platform}`), device: grant.client_name })
      : grant.client_name;
  }

  /** "Allowed 28 October 2026 · Last used today, 14:32", in the reader's language. */
  function noteOf(grant: Grant): string {
    const day = formatLongDay(grant.created_at, messages.locale);
    const use = grantUse(grant.last_used_at, messages.locale);
    switch (use.kind) {
      case 'never':
        return t('app.grants.allowed_unused', { day });
      case 'today':
        return t('app.grants.allowed_used_today', { day, time: formatTime(use.at, messages.locale) });
      case 'days':
        return t('app.grants.allowed_used', { day, used: use.phrase });
      case 'day':
        return t('app.grants.allowed_used', { day, used: formatLongDay(use.at, messages.locale) });
    }
  }

  /** The app the reader is asked about: withdrawing is final - it has to be allowed again (§E0). */
  let asking = $state<{ id: string; name: string; isApp: boolean } | undefined>(undefined);

  async function withdraw(grantId: string): Promise<void> {
    failure = undefined;
    try {
      await consent.withdraw(grantId);
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <Stack gap="300">
    <SettingsHead row="grants" />

    <Stack gap="200">
      <p class="prose">{t('app.grants.intro')}</p>

      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

      {#if rows.length === 0}
        <p class="prose">{t('app.grants.none')}</p>
      {:else}
        <ul class="list" aria-label={t('app.grants.title')}>
          {#each rows as grant (grant.id)}
            {@const name = nameOf(grant)}
            <li>
              {#if grant.app_platform}
                <!-- The installed app's own icon: the mark on the brand tile, as the launcher shows it. -->
                <span class="icon" aria-hidden="true"><Logo size="md" /></span>
              {:else}
                <span class="mark" aria-hidden="true">{initialsOf(grant.client_name)}</span>
              {/if}
              <div class="grow">
                <span class="name">{name}</span>
                <span class="note">{noteOf(grant)}</span>
              </div>
              <Button size="sm" tone="secondary" onclick={() => (asking = { id: grant.id, name, isApp: !!grant.app_platform })}>
                {grant.app_platform ? t('app.grants.sign_out') : t('app.grants.withdraw')}
              </Button>
            </li>
          {/each}
        </ul>
      {/if}
    </Stack>
  </Stack>

  <FinalActDialog
    isOpen={asking !== undefined}
    title={t(asking?.isApp ? 'app.grants.sign_out_confirm_title' : 'app.grants.withdraw_confirm_title', { app: asking?.name ?? '' })}
    body={t(asking?.isApp ? 'app.grants.sign_out_confirm_body' : 'app.grants.withdraw_confirm_body', { app: asking?.name ?? '' })}
    confirmLabel={t(asking?.isApp ? 'app.grants.sign_out' : 'app.grants.withdraw')}
    busyLabel={t(asking?.isApp ? 'app.grants.signing_out' : 'app.grants.withdrawing')}
    onConfirm={async () => {
      if (asking) await withdraw(asking.id);
    }}
    onClose={() => (asking = undefined)}
  />
{/if}

<style>
  .prose { margin: 0; max-inline-size: 72ch; color: var(--text-secondary); }

  .failure { margin: 0; color: var(--text-danger); }

  .list {
    display: flex;
    flex-direction: column;
    margin: 0;
    padding: 0;
    list-style: none;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    overflow: hidden;
  }

  /* On a phone the action wraps under the name rather than crushing it. */
  li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-150);
    padding: var(--sp-150);
    background: var(--bg-surface);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  li:last-child { border-block-end: 0; }

  .grow { flex: 1 1 0; min-inline-size: min(100%, 16ch); }

  .mark {
    display: inline-grid;
    place-items: center;
    flex: none;
    inline-size: var(--sp-400);
    block-size: var(--sp-400);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
  }

  /* The launcher's icon: the outline turns to the tile's own colour, as `app-icon.svg` draws it. */
  .icon {
    display: inline-grid;
    place-items: center;
    flex: none;
    inline-size: var(--sp-500);
    block-size: var(--sp-500);
    border-radius: var(--r-md);
    background: var(--brand-tile);
    --brand-outline: var(--brand-tile-outline);
  }

  .name { display: block; font-weight: var(--fw-medium); overflow-wrap: anywhere; }

  .note {
    display: block;
    margin-block-start: var(--sp-050);
    font-size: var(--fs-075);
    color: var(--text-subtle);
  }

</style>
