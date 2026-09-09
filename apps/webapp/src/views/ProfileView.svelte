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
    canBeCleared,
    categoriesOf,
    channelsOf,
    clearedOr,
    isAlwaysOn,
    knownZones,
    localesOf,
    preferenceFor,
  } from '../lib/data/preferences.ts';
  import { mfa } from '../lib/data/mfa.svelte.ts';
  import { consent } from '../lib/data/consent.svelte.ts';
  import { sessions } from '../lib/data/sessions.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import TotpEnrollment from '../lib/frame/TotpEnrollment.svelte';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { session } from '../lib/session.svelte.ts';

  const account = $derived(actor.account);
  const accountId = $derived(account?.id);

  $effect(() => {
    const wanted = accountId;
    if (!wanted) return;
    return untrack(() => preferences.open(wanted));
  });

  // The sessions are the account's own and take no parameter, so the read starts with the screen
  // rather than with an identifier arriving.
  $effect(() => untrack(() => sessions.open()));
  // The grants sit beside the sessions because they are the same question asked about apps:
  // what is currently able to act as me, and how do I stop it.
  $effect(() => untrack(() => consent.openGrants()));

  let grantFailure = $state<string | undefined>(undefined);

  async function withdraw(grantId: string): Promise<void> {
    grantFailure = undefined;
    try {
      await consent.withdraw(grantId);
    } catch (error) {
      grantFailure = renderProblem(error as never, messages).message;
    }
  }

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
      const clearedWeek = clearedOr(weekStart) === '';
      await preferences.setAccount(accountId, {
        // The empty string, not null: a present-but-nil entry reads as "not sent" and the value
        // stays. Walked against a running server, both ways round.
        locale: clearedOr(locale),
        time_zone: clearedOr(zone),
        // …except this one, which cannot be cleared at all — so an empty choice sends nothing
        // rather than sending something that is refused.
        ...(clearedWeek ? {} : { week_start: clearedOr(weekStart) as never }),
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

  const held = $derived(sessions.state);

  let disablePassword = $state('');
  let mfaNotice = $state<string | undefined>(undefined);

  /**
   * Takes the second factor off, with the password afresh.
   *
   * The one case where being signed in is not enough: a stolen session removing the factor is
   * exactly the attack the factor exists against (`security.md` §5).
   */
  async function disableFactor(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const password = disablePassword;
    disablePassword = '';
    if (await mfa.disable(password)) mfaNotice = t('app.mfa.disabled');
  }

  /** An instant as this reader reads one: their locale, their clock (`i18n-l10n.md` §4). */
  function when(at: string): string {
    return formatDateTime(at, messages.locale);
  }

  /**
   * Ends one session.
   *
   * Ending the current one is a sign-out and is treated as one: the list would otherwise be
   * re-read with a credential that has just stopped working, and the reader would meet a refusal
   * instead of the sign-in screen.
   */
  async function endOne(id: string, isCurrent: boolean): Promise<void> {
    failure = undefined;
    try {
      await sessions.end(id);
      if (isCurrent) await session.signOut();
    } catch (error) {
      failure = renderProblem(error as never, messages);
    }
  }

  /** Ends every session, this one last by consequence rather than by order. */
  async function endEverywhere(): Promise<void> {
    failure = undefined;
    try {
      await sessions.endAll();
    } catch (error) {
      failure = renderProblem(error as never, messages);
    }
    // Whatever the server managed, this tab is holding a credential it has been told to stop
    // trusting. Discarding it is not conditional on the call having succeeded.
    await session.signOut();
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
      {#if locales.length > 0}
        <Select
          label={t('app.profile.language')}
          hint={t('app.profile.language_hint')}
          bind:value={locale}
          placeholder={t('app.profile.use_workspace')}
          options={locales.map((each) => ({ value: each.locale, label: each.locale }))}
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
          placeholder={t('app.profile.use_workspace')}
          options={zones.map((each) => ({ value: each, label: each }))}
        />
      {:else}
        <!-- A browser that will not list the zones still lets somebody type one, and the server
             refuses an unknown name. A worse offer, not a broken one. -->
        <Input label={t('app.profile.zone')} hint={t('app.profile.zone_free')} bind:value={zone} />
      {/if}

      <!-- The empty choice is offered only while there is nothing to clear. Once a day is chosen
           this version cannot un-choose it — `""` is refused and `null` reads as "not sent" — and a
           control that offered the choice would be offering one that quietly does nothing. -->
      <Select
        label={t('app.profile.week_start')}
        hint={weekStart === '' || canBeCleared('week_start')
          ? t('app.profile.week_start_hint')
          : t('app.profile.week_start_fixed')}
        bind:value={weekStart}
        placeholder={weekStart === '' ? t('app.profile.use_workspace') : undefined}
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
      <h2 class="section">{t('app.mfa.title')}</h2>
      <!-- Whether one is armed is not something this client is told: no read answers it, and
           inferring it from a sign-in that did not ask for a code would be inferring from an
           absence. So the panel offers enrolment, and the server refuses one that is already
           armed — in its own words, which is the honest answer rather than a guess. -->
      <TotpEnrollment onarmed={() => (mfaNotice = t('app.mfa.armed'))} />

      {#if mfaNotice}<p class="quiet">{mfaNotice}</p>{/if}

      <details>
        <summary>{t('app.mfa.disable')}</summary>
        <Stack gap="150">
          <p class="quiet">{t('app.mfa.disable_hint')}</p>
          <form onsubmit={disableFactor}>
            <Stack gap="150">
              <Input
                label={t('app.step_up.password_label')}
                bind:value={disablePassword}
                type="password"
                autocomplete="current-password"
                spellcheck={false}
                isRequired
              />
              <div>
                <Button
                  type="submit"
                  tone="danger"
                  isBusy={mfa.isWorking}
                  busyLabel={t('app.mfa.disabling')}
                >
                  {t('app.mfa.disable')}
                </Button>
              </div>
            </Stack>
          </form>
        </Stack>
      </details>
    </Stack>

    <Stack gap="150">
      <h2 class="section">{t('app.sessions.title')}</h2>
      <p class="quiet">{t('app.sessions.intro')}</p>

      {#if held === undefined || held.status === 'loading' || held.status === 'idle'}
        <div aria-busy="true"><Skeleton lines={2} /></div>
      {:else if held.status === 'failed'}
        <ErrorState
          title={renderProblem(held.error, messages).message}
          retryLabel={t('app.retry')}
          onRetry={() => sessions.open()}
        />
      {:else if sessions.all.length === 0}
        <p class="quiet">{t('app.sessions.none')}</p>
      {:else}
        <ul class="rows">
          {#each sessions.all as held (held.id)}
            <li>
              <div class="row">
                <div>
                  <span class="category">
                    {held.user_agent || t('app.sessions.unknown_client')}
                    {#if held.current} · {t('app.sessions.this_device')}{/if}
                  </span>
                  <span class="meta">
                    {t('app.sessions.created')} {when(held.created_at)}
                    {#if held.last_used_at}
                      · {t('app.sessions.last_used')} {when(held.last_used_at)}
                    {:else}
                      · {t('app.sessions.never_used')}
                    {/if}
                    {#if held.ip_class} · {held.ip_class}{/if}
                  </span>
                </div>
                <div class="switches">
                  <Button
                    tone="danger"
                    size="sm"
                    onclick={() => void endOne(held.id, held.current)}
                  >
                    {held.current ? t('app.sessions.end_this') : t('app.sessions.end')}
                  </Button>
                </div>
              </div>
            </li>
          {/each}
        </ul>

        <div>
          <!-- Said before it is pressed, because it ends this session too: a control whose
               consequence is "you are about to be signed out" has to say so where the finger is. -->
          <p class="quiet">{t('app.sign_out.everywhere_warning')}</p>
          <Button tone="danger" onclick={() => void endEverywhere()}>
            {t('app.sign_out.everywhere')}
          </Button>
        </div>
      {/if}
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

    <Stack gap="150">
      <h2 class="section">{t('app.grants.title')}</h2>
      <p class="quiet">{t('app.grants.intro')}</p>
      {#if grantFailure}<p class="failure">{grantFailure}</p>{/if}
      <ul class="rows">
        {#each consent.grants as grant (grant.id)}
          <li class="row">
            <span>
              <span class="category">{grant.client_name}</span>
              <!-- The scopes as the app holds them. Sentences where this build knows them, and the
                   identifier where it does not — the same rule as the consent screen. -->
              <span class="quiet small">{grant.scopes.join(', ')}</span>
            </span>
            <Button size="sm" tone="subtle" onclick={() => void withdraw(grant.id)}>
              {t('app.grants.withdraw')}
            </Button>
          </li>
        {:else}
          <li class="quiet">{t('app.grants.none')}</li>
        {/each}
      </ul>
      <p class="quiet small">{t('app.grants.next_request')}</p>
    </Stack>

    <Stack gap="050">
      <h2 class="section">{t('app.tokens.title')}</h2>
      <p class="quiet">{t('app.tokens.from_profile')}</p>
      <!-- A link rather than a panel: minting a credential is its own screen, and burying it under
           a language chooser would put a one-time secret in the middle of a settings page. -->
      <p><a href="/profile/tokens">{t('app.tokens.open')}</a></p>
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

  .label { font-size: var(--fs-075); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; }

  .failure { margin: 0; color: var(--text-danger); }
</style>
