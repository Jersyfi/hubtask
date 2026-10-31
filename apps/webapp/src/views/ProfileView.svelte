<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *You*: the first screen of Your settings (ADR-0065 decision 3) and the bottom bar's *You* -
  // what the others call this reader, and how the product speaks to them.
  //
  // **The name is the person's own** (`PATCH /accounts/me`), shown with the avatar: the picture the
  // person chose, or the initials of the name. The name saves with the language under the one save
  // bar, because both are a form somebody fills in and leaves; the picture is written the moment it
  // is chosen (PictureField).
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
  // **One more thing is the person's own here**: keeping their content out of AI, beside how the
  // product speaks to them, because it is a choice about themselves rather than about a device.
  //
  // **And the moments**: whether the product marks them is the person's, on every device, so the
  // switch stands here and not with the theme.
  //
  // **Nothing else is under it.** The second factor, the sessions, the devices, the theme, the
  // notifications, the apps and the tokens are each a row of the section's column with an address
  // of its own.
  //
  // **Nothing is compiled in.** The languages are the manifest's.
  //
  // **A change shows the save bar, and only a change** (design-system.md §4, case 4; UC-WRK-32
  // check 3): "2 changes · Discard · Save" appears with the first change, saving is busy in place
  // and confirms as a toast. The write is a merge-patch without a version, so it sends only what
  // changed - a second device's change to another field survives - and the form is not re-seeded
  // from the account while it holds a change, which would wipe the count from under the reader.

  import { untrack } from 'svelte';

  import { Avatar, Badge, EmptyState, Input, SaveBar, Select, Stack } from '@hubtask/design-system/components';
  import type { OwnSettings } from '@hubtask/sync-engine';

  import SettingsHead from '../lib/frame/SettingsHead.svelte';
  import AiObjectionSection from '../lib/profile/AiObjectionSection.svelte';
  import DelegationSection from '../lib/profile/DelegationSection.svelte';
  import PictureField from '../lib/profile/PictureField.svelte';
  import CelebrationsSwitch from '../lib/profile/CelebrationsSwitch.svelte';
  import AddressForm from '../lib/people/AddressForm.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { preferences } from '../lib/data/preferences.svelte.ts';
  import { WEEK_STARTS, clearedOr, inForce, zoneOptions, localesOf, withWorkspaceChoice } from '../lib/data/preferences.ts';
  import { engine } from '../lib/data/engine.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { languageName } from '../lib/i18n/locale.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { signInRules } from '../lib/data/signinrules.svelte.ts';

  const account = $derived(actor.account);

  // Where each of the person's settings comes from, as the catalogue resolves it (model.md §4):
  // the server's answer, re-read when a save invalidates `/accounts`.
  let settings = $state<OwnSettings>();
  $effect(() => untrack(() => engine.subscribe<OwnSettings>({ path: '/accounts/me/settings' }, (next) => {
    settings = next.status === 'ready' ? next.data : settings;
  })));
  const sourceOf = (key: string, label: (value: string) => string) => {
    const held = inForce(settings?.data, `localisation.${key}`);
    if (!held) return undefined;
    return { source: held.source, value: held.value === undefined ? t('app.profile.source_default_value') : label(held.value) };
  };

  // Whether the password is a way in here decides whether an address may be added (identity.md
  // §13); the sign-in rules say it, fallback included.
  $effect(() => signInRules.read());
  const accountId = $derived(account?.id);

  const locales = $derived(localesOf(manifest.value));
  // Whatever the account holds has to be selectable, alias or not — otherwise a zone that is
  // set reads as "use the workspace's".
  const zones = $derived(zoneOptions(account?.time_zone));

  let displayName = $state('');
  let locale = $state('');
  let zone = $state('');
  let weekStart = $state('');
  let isSaving = $state(false);
  let nameError = $state<string | undefined>(undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /** What the form was last seeded with - the account's values - which a change is counted against. */
  let seeded = $state({ displayName: '', locale: '', zone: '', weekStart: '' });
  /** The fields that differ from what was seeded, by the name the write takes. */
  const changed = $derived(
    [
      displayName.trim() !== seeded.displayName ? 'display_name' : undefined,
      locale !== seeded.locale ? 'locale' : undefined,
      zone !== seeded.zone ? 'time_zone' : undefined,
      weekStart !== seeded.weekStart ? 'week_start' : undefined,
    ].filter((each): each is string => each !== undefined),
  );

  // Seeded from the account, and re-seeded when it arrives or changes - but only while the reader
  // holds no change of their own: an account re-read in the middle of an edit would otherwise
  // throw the edit away. In an effect rather than a `$derived`, because these are what somebody is
  // typing into.
  $effect(() => {
    const next = held();
    untrack(() => {
      if (changed.length > 0) return;
      seed(next);
    });
  });

  /** What the account holds, in the form's terms. */
  function held() {
    return {
      displayName: account?.display_name ?? '',
      locale: account?.locale ?? '',
      zone: account?.time_zone ?? '',
      weekStart: account?.week_start ?? '',
    };
  }

  function seed(values: ReturnType<typeof held>) {
    seeded = values;
    displayName = values.displayName;
    locale = values.locale;
    zone = values.zone;
    weekStart = values.weekStart;
  }

  function discard() {
    failure = undefined;
    nameError = undefined;
    seed(held());
  }

  async function save() {
    if (!accountId) return;
    if (changed.length === 0) return;
    failure = undefined;
    // Said at the field and sent nowhere: an empty name is not a way back to a default, there is
    // none, and the server would refuse it the same way.
    nameError = displayName.trim() === '' ? t('app.profile.display_name_empty') : undefined;
    if (nameError) return;
    isSaving = true;
    const sent = { displayName: displayName.trim(), locale, zone, weekStart };
    const prefers = changed.some((field) => field !== 'display_name');
    try {
      // The name first, through its own write: a name refused stops the save before the language
      // changes under the reader's eyes.
      if (changed.includes('display_name')) {
        await actor.rename(sent.displayName);
        seeded = { ...seeded, displayName: sent.displayName };
      }
      // Only what changed: the write merges, and a field sent unchanged would overwrite another
      // device's change to it. The empty string, not null: it is what every server version clears
      // on, where a null was read as "not sent" before 0.9.0.
      if (prefers) {
        await preferences.setAccount(accountId, {
          ...(changed.includes('locale') ? { locale: clearedOr(sent.locale) } : {}),
          ...(changed.includes('time_zone') ? { time_zone: clearedOr(sent.zone) } : {}),
          ...(changed.includes('week_start') ? { week_start: clearedOr(sent.weekStart) as never } : {}),
        });
      }
      seeded = sent;
      // The frame applies the language itself once the account re-reads; this only says it landed.
      toast.show({ text: t('app.profile.saved'), tone: 'success' });
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
        <!-- The avatar beside the field: what the others see - the picture, or the initials the
             name makes. -->
        <div class="who">
          <Avatar name={displayName.trim() || account.display_name} src={accounts.pictureOf(account.id)} />
          <div class="name">
            <Input
              label={t('app.profile.display_name')}
              data-setting="name"
              hint={t('app.profile.display_name_hint')}
              bind:value={displayName}
              error={nameError}
              maxlength={200}
              autocomplete="name"
              isRequired
            />
          </div>
        </div>

        <PictureField />


        {#if locales.length > 0}
          <Select
            label={t('app.profile.language')}
            data-setting="language"
            hint={t('app.profile.language_hint')}
            bind:value={locale}
            options={withWorkspaceChoice(
              locales.map((each) => ({ value: each.locale, label: languageName(each.locale, messages.locale) })),
              t('app.profile.use_workspace'),
            )}
          />
          {@render inForceLine(sourceOf('locale', (value) => languageName(value, messages.locale)))}
        {:else}
          <!-- An installation that declares no locales has none to choose between, and an empty
               dropdown says that badly. Found by reading a real manifest, which reports none. -->
          <Stack gap="050">
            <span class="label" data-setting="language">{t('app.profile.language')}</span>
            <p class="quiet">{t('app.profile.no_locales')}</p>
          </Stack>
        {/if}

        {#if zones.length > 0}
          <Select
            label={t('app.profile.zone')}
            data-setting="zone"
            hint={t('app.profile.zone_hint')}
            bind:value={zone}
            options={withWorkspaceChoice(
              zones.map((each) => ({ value: each, label: each })),
              t('app.profile.use_workspace'),
            )}
          />
          {@render inForceLine(sourceOf('time_zone', (value) => value))}
        {:else}
          <!-- A browser that will not list the zones still lets somebody type one, and the server
               refuses an unknown name. A worse offer, not a broken one. -->
          <Input label={t('app.profile.zone')} data-setting="zone" hint={t('app.profile.zone_free')} bind:value={zone} />
        {/if}

        <Select
          label={t('app.profile.week_start')}
          data-setting="week-start"
          hint={t('app.profile.week_start_hint')}
          bind:value={weekStart}
          options={withWorkspaceChoice(
            WEEK_STARTS.map((each) => ({ value: each, label: t(`app.profile.week_${each}`) })),
            t('app.profile.use_workspace'),
          )}
        />
        {@render inForceLine(sourceOf('week_start', (value) => t(`app.profile.week_${value}`)))}

        {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
      </Stack>
      <SaveBar
        label={t('app.save_bar.label')}
        changesLabel={changed.length > 0 ? t('app.save_bar.changes', { count: String(changed.length) }) : undefined}
        discardLabel={t('app.save_bar.discard')}
        saveLabel={t('app.profile.save')}
        isSaving={isSaving}
        savingLabel={t('app.workspace.saving')}
        onDiscard={discard}
        onSave={() => void save()}
      />
    </form>

    <!-- The moments: the account's, so here rather than on *On this device* (ADR-0043). A switch
         of its own outside the form, written at once as every switch outside a form is. -->
    <CelebrationsSwitch />

    <!-- Keeping one's own content out of AI: the person's own choice, so on the profile; absent
         where there is nothing to choose (UC-PRV-05 check 8). -->
    <AiObjectionSection />
    <!-- Who decides the person's approvals while they are away, where the service desk is on (UC-SVC-10). -->
    {#if manifest.value?.modules_on?.includes('SERVICE_DESK')}<DelegationSection />{/if}
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

{#snippet inForceLine(held: { source: string; value: string } | undefined)}
  {#if held}
    <p class="in-force" data-source={held.source}>
      <span>{t('app.profile.in_force', { value: held.value })}</span>
      <Badge tone={held.source === 'PERSON' ? 'info' : 'neutral'}>{t(`app.profile.source_${held.source.toLowerCase()}`)}</Badge>
    </p>
  {/if}
{/snippet}

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
    box-shadow: var(--shadow-raised);
  }

  /* The avatar beside the field, at the field's own line, and above it where a phone has no room. */
  .who {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-150);
  }

  .name { flex: 1 1 24ch; min-inline-size: 0; }

  .label { font-size: var(--fs-075); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); }
  .in-force { margin: 0; display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); color: var(--text-secondary); font-size: var(--fs-075); }

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  /* The form's own measure, as the panel above carries it. */
  .measured { max-inline-size: 52ch; }

  .failure { margin: 0; color: var(--text-danger); }
</style>
