<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // How the product speaks to this reader — the first screen of Your settings (ADR-0065
  // decision 3), and nothing else.
  //
  // **The client reads these three, and this is the one screen that sets them.** It does the
  // setting only — the frame still applies the language and the
  // direction, in the one place it applies them. A write names `/accounts`, the frame's read of
  // `/accounts/me` is watched, and the engine re-reads it: the page changes language without a
  // reload and without a second module deciding which language it is.
  //
  // **Clearing is not setting to nothing.** An empty value means the workspace's own applies again,
  // and the field says that rather than going blank and leaving somebody to guess.
  //
  // **Nothing else is under it.** The second factor, the sessions, the devices, the theme, the
  // notifications, the apps and the tokens are each a row of the section's column with an address
  // of its own.
  //
  // **Nothing is compiled in.** The languages are the manifest's.

  import { Button, EmptyState, Input, Select, Stack } from '@hubtask/design-system/components';

  import SettingsHead from '../lib/frame/SettingsHead.svelte';
  import AddressForm from '../lib/people/AddressForm.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { preferences } from '../lib/data/preferences.svelte.ts';
  import { WEEK_STARTS, clearedOr, zoneOptions, localesOf, withWorkspaceChoice } from '../lib/data/preferences.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { languageName } from '../lib/i18n/locale.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { signInRules } from '../lib/data/signinrules.svelte.ts';

  const account = $derived(actor.account);

  // Whether the password is a way in here decides whether an address may be added (identity.md
  // §13); the sign-in rules say it, fallback included.
  $effect(() => signInRules.read());
  const accountId = $derived(account?.id);

  const locales = $derived(localesOf(manifest.value));
  // Whatever the account holds has to be selectable, alias or not — otherwise a zone that is
  // set reads as "use the workspace's".
  const zones = $derived(zoneOptions(account?.time_zone));

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
        // The empty string, not null: it is what every server version clears on, where a null
        // was read as "not sent" before 0.9.0.
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
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <Stack gap="300">
    <SettingsHead row="profile" />

    <!-- Where the tour's `profile` step points. It goes to `/profile`, which is the
         section's first screen, and what is on the next row of the column its sentence names. -->
    <form class="panel" data-tour="profile" onsubmit={(event) => { event.preventDefault(); void save(); }}>
      <Stack gap="200">
        {#if locales.length > 0}
          <Select
            label={t('app.profile.language')}
            hint={t('app.profile.language_hint')}
            bind:value={locale}
            options={withWorkspaceChoice(
              locales.map((each) => ({ value: each.locale, label: languageName(each.locale, messages.locale) })),
              t('app.profile.use_workspace'),
            )}
          />
        {:else}
          <!-- An installation that declares no locales has none to choose between, and an empty
               dropdown says that badly. Found by reading a real manifest, which reports none. -->
          <Stack gap="050">
            <span class="label">{t('app.profile.language')}</span>
            <p class="quiet">{t('app.profile.no_locales')}</p>
          </Stack>
        {/if}

        {#if zones.length > 0}
          <Select
            label={t('app.profile.zone')}
            hint={t('app.profile.zone_hint')}
            bind:value={zone}
            options={withWorkspaceChoice(
              zones.map((each) => ({ value: each, label: each })),
              t('app.profile.use_workspace'),
            )}
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
          options={withWorkspaceChoice(
            WEEK_STARTS.map((each) => ({ value: each, label: t(`app.profile.week_${each}`) })),
            t('app.profile.use_workspace'),
          )}
        />

        {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
        {#if notice}<p class="quiet">{notice}</p>{/if}

        <div>
          <Button type="submit" isBusy={isSaving} busyLabel={t('app.workspace.saving')}>
            {t('app.profile.save')}
          </Button>
        </div>
      </Stack>
    </form>

    {#if account.sign_in_name && !account.email && signInRules.isSettled}
      <!-- A person without a mail address may add one (UC-ID-20 check 7): drawn exactly where the
           server would take it - a sign-in name and no address, in a workspace whose password is a
           way in. Where it is not, the form is absent and the reason stands in its place. -->
      <Stack gap="150">
        <h2 class="section">{t('app.profile.address_title')}</h2>
        {#if signInRules.hasPassword}
          <p class="quiet">{t('app.profile.address_intro', { name: account.sign_in_name })}</p>
          <div class="panel measured">
            <AddressForm accountId={account.id} hint={t('app.people.address_hint_own')} />
          </div>
        {:else}
          <p class="quiet">{t('app.profile.address_needs_password', { name: account.sign_in_name })}</p>
        {/if}
      </Stack>
    {/if}
  </Stack>
{/if}

<style>
  /* A form is neither prose nor a table, and it is the third case of ADR-0065 decision 2: an
     input as wide as the region is a target nobody aims at, so the fields carry a measure of
     their own while the lists and tables of the section take the width. */
  form { margin: 0; max-inline-size: 52ch; }

  /* The surface a form stands on, as the other screens of the section draw one: a standalone
     element in the sense of design-system.md rule 1, on the frame's canvas. */
  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .label { font-size: var(--fs-075); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  /* The form's own measure, as the panel above carries it. */
  .measured { max-inline-size: 52ch; }

  .failure { margin: 0; color: var(--text-danger); }
</style>
