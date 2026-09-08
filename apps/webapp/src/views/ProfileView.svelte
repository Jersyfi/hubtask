<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // How the product speaks to this reader, and what it tells them about.
  //
  // **The client has read these three since F1-08 and could never set them.** This is the screen
  // that closes that, and it does the setting only — the frame still applies the language and the
  // direction, in the one place it applies them. A write names `/accounts`, the frame's read of
  // `/accounts/me` is watched, and the engine re-reads it: the page changes language without a
  // reload and without a second module deciding which language it is.
  //
  // **Clearing is not setting to nothing.** An empty value means the workspace's own applies again,
  // and the field says that rather than going blank and leaving somebody to guess.
  //
  // **The theme is not here, and the screen says where it is.** ADR-0043 made it a property of the
  // device rather than of the account, and a reader who looks for it and finds nothing learns
  // nothing — so they find a sentence instead.
  //
  // **Nothing is compiled in.** The languages, the categories and the channels are the manifest's.
  // A category this version has no phrase for still renders, because `t` humanises an unknown code.

  import { untrack } from 'svelte';

  import {
    Button,
    Checkbox,
    EmptyState,
    ErrorState,
    Input,
    Select,
    Skeleton,
    Stack,
  } from '@hubtask/design-system/components';

  import { actor } from '../lib/data/account.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { preferences } from '../lib/data/preferences.svelte.ts';
  import {
    WEEK_STARTS,
    categoriesOf,
    channelsOf,
    clearedOr,
    isAlwaysOn,
    knownZones,
    localesOf,
    preferenceFor,
  } from '../lib/data/preferences.ts';
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

  const locales = $derived(localesOf(manifest.value));
  const zones = $derived(knownZones());
  const categories = $derived(categoriesOf(manifest.value));
  const channels = $derived(channelsOf(manifest.value));
  const rows = $derived(accountId ? preferences.of(accountId) : []);
  const reading = $derived(accountId ? preferences.stateOf(accountId) : undefined);

  let locale = $state('');
  let zone = $state('');
  let weekStart = $state('');
  let isSaving = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let notice = $state<string | undefined>(undefined);

  // Seeded from the account, and re-seeded when it arrives or changes. In an effect rather than a
  // `$derived`, because these are what somebody is typing into.
  $effect(() => {
    locale = account?.locale ?? '';
    zone = account?.time_zone ?? '';
    weekStart = account?.week_start ?? '';
  });

  async function save() {
    if (!accountId) return;
    isSaving = true;
    failure = undefined;
    notice = undefined;
    try {
      await preferences.setAccount(accountId, {
        // Null, not an empty string: "no preference" and "a value that happens to be empty" are
        // different statements, and the schema types all three as nullable for that reason.
        locale: clearedOr(locale),
        time_zone: clearedOr(zone),
        week_start: clearedOr(weekStart) as never,
      });
      notice = t('app.profile.saved');
      // The frame applies the language itself once the account re-reads; this only says it landed.
      announcer.say(t('app.profile.saved'));
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isSaving = false;
    }
  }

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
    } catch (error) {
      failure = renderProblem(error as never, messages);
    }
  }
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <Stack gap="300">
    <h1 class="name">{t('app.profile.title')}</h1>

    <Stack gap="150">
      <Select
        label={t('app.profile.language')}
        hint={t('app.profile.language_hint')}
        bind:value={locale}
        placeholder={t('app.profile.use_workspace')}
        options={locales.map((each) => ({ value: each.locale, label: each.locale }))}
      />

      {#if zones.length > 0}
        <Select
          label={t('app.profile.zone')}
          hint={t('app.profile.zone_hint')}
          bind:value={zone}
          placeholder={t('app.profile.use_workspace')}
          options={zones.map((each) => ({ value: each, label: each }))}
        />
      {:else}
        <!-- A browser that will not list the zones still lets somebody type one, and the server
             refuses an unknown name. A worse offer, not a broken one. -->
        <Input label={t('app.profile.zone')} hint={t('app.profile.zone_free')} bind:value={zone} />
      {/if}

      <Select
        label={t('app.profile.week_start')}
        hint={t('app.profile.week_start_hint')}
        bind:value={weekStart}
        placeholder={t('app.profile.use_workspace')}
        options={WEEK_STARTS.map((each) => ({ value: each, label: t(`app.profile.week_${each}`) }))}
      />

      {#if failure}<p class="failure">{failure.message}</p>{/if}
      {#if notice}<p class="quiet">{notice}</p>{/if}

      <div>
        <Button isBusy={isSaving} busyLabel={t('app.workspace.saving')} onclick={() => void save()}>
          {t('app.profile.save')}
        </Button>
      </div>
    </Stack>

    <Stack gap="150">
      <h2 class="section">{t('app.profile.theme')}</h2>
      <!-- Not a control. ADR-0043 put the theme on the device, and a reader who looks for it here
           learns where it is rather than finding nothing. -->
      <p class="quiet">{t('app.profile.theme_elsewhere')}</p>
    </Stack>

    <Stack gap="150">
      <h2 class="section">{t('app.profile.notifications')}</h2>
      <p class="quiet">{t('app.profile.notifications_hint')}</p>

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
        <ul class="rows">
          {#each categories as category (category)}
            {#each channels as channel (channel)}
              {@const row = preferenceFor(rows, category, channel)}
              {@const alwaysOn = isAlwaysOn(category)}
              <li>
                <div class="row">
                  <div>
                    <!-- A phrase per category the catalogue knows, and `humanise` for one it does
                         not: an installation that tells people about something newer still reads
                         readably rather than showing a key. The **list** is the manifest's; only
                         the wording is the catalogue's. -->
                    <span class="category">{t(`app.profile.category_${category}`)}</span>
                    <span class="meta">
                      {t(`app.profile.channel_${channel}`)}
                      {#if row?.is_default} · {t('app.profile.is_default')}{/if}
                    </span>
                  </div>
                  <div class="switches">
                    <Checkbox
                      label={t('app.profile.enabled')}
                      checked={row?.enabled ?? true}
                      disabledReason={alwaysOn ? t('app.profile.always_on') : undefined}
                      onchange={(event: Event) =>
                        void setRow(category, channel, {
                          enabled: (event.currentTarget as HTMLInputElement).checked,
                        })}
                    />
                    <Checkbox
                      label={t('app.profile.include_title')}
                      hint={t('app.profile.include_title_hint')}
                      checked={row?.include_title ?? false}
                      onchange={(event: Event) =>
                        void setRow(category, channel, {
                          include_title: (event.currentTarget as HTMLInputElement).checked,
                        })}
                    />
                  </div>
                </div>
              </li>
            {/each}
          {/each}
        </ul>
      {/if}
    </Stack>
  </Stack>
{/if}

<style>
  .name {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
  }

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .rows { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-150); }

  .row { display: flex; flex-wrap: wrap; align-items: start; justify-content: space-between; gap: var(--sp-150); }

  .category { display: block; font-weight: var(--fw-semibold); }

  .meta { display: block; color: var(--text-secondary); font-size: var(--fs-075); max-width: 48ch; }

  .switches { display: flex; flex-wrap: wrap; gap: var(--sp-200); }

  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; }

  .failure { margin: 0; color: var(--text-danger); }
</style>
