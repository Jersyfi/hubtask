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
  // **A settings page** (page-system §9.5): *Name*, *Language and time*, *Moments* and, where they
  // apply, *AI*, *While I am away* and *A mail address*, each at its own address
  // (`/profile/you/:section`); at `/profile` the list, with the first section beside it where there
  // is room.
  //
  // **A change shows the save bar, and only a change** (design-system.md §4, case 4; UC-WRK-32
  // check 3): "2 changes · Discard · Save" appears with the first change, saving is busy in place
  // and confirms as a toast. The write is a merge-patch without a version, so it sends only what
  // changed - a second device's change to another field survives - and the form is not re-seeded
  // from the account while it holds a change, which would wipe the count from under the reader.

  import { untrack } from 'svelte';

  import { Avatar, Badge, EmptyState, Input, SaveBar, Select, SettingRow, SettingsLayout, SettingsSection, type SectionLink } from '@hubtask/design-system/components';
  import ZonePicker from '../lib/forms/ZonePicker.svelte';
  import type { OwnSettings } from '@hubtask/sync-engine';

  import { page } from '../lib/frame/page.svelte.ts';
  import { profileHead, profileTitle } from '../lib/profile/profilehead.ts';
  import AiObjectionSection from '../lib/profile/AiObjectionSection.svelte';
  import DelegationSection from '../lib/profile/DelegationSection.svelte';
  import PictureField from '../lib/profile/PictureField.svelte';
  import CelebrationsSwitch from '../lib/profile/CelebrationsSwitch.svelte';
  import UndeliverableNotice from '../lib/profile/UndeliverableNotice.svelte';
  import AddressForm from '../lib/people/AddressForm.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { aiObjection } from '../lib/data/aiobjection.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { preferences } from '../lib/data/preferences.svelte.ts';
  import { WEEK_STARTS, clearedOr, inForce, languageInForce, zoneOptions, localesOf, withWorkspaceChoice, workspaceChoiceLabel } from '../lib/data/preferences.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { languageName } from '../lib/i18n/locale.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { signInRules } from '../lib/data/signinrules.svelte.ts';

  interface Props {
    /** The section the address names; none is the list. */
    section?: string;
    /** Moves between the sections, replacing the address. */
    onreplace?: (path: string) => void;
  }
  const { section, onreplace }: Props = $props();

  const BASE = '/profile/you';
  const title = $derived(profileTitle('profile'));
  $effect(() => page.entitle(title));

  const account = $derived(actor.account);

  // Where each of the person's settings comes from, as the catalogue resolves it (model.md §4):
  // the server's answer, re-read when a save invalidates `/accounts`.
  let settings = $state<OwnSettings>();
  $effect(() => untrack(() => engine.subscribe<OwnSettings>({ path: '/accounts/me/settings' }, (next) => {
    settings = next.status === 'ready' ? next.data : settings;
  })));
  // The workspace's own value of each, for the choice that leaves it to the workspace (§8.8):
  // what it decided, else what the installation decided for it - never a guess.
  $effect(() => untrack(() => workspace.open()));
  const ownDefaults = $derived(workspace.workspace);
  const leaveTo = (value: string | undefined) =>
    workspaceChoiceLabel(t('app.profile.use_workspace'), (named) => t('app.profile.use_workspace_named', { value: named }), value);
  const workspaceLocale = $derived(ownDefaults ? leaveTo(languageName(ownDefaults.default_locale, messages.locale)) : t('app.profile.use_workspace'));
  const workspaceZone = $derived(leaveTo(ownDefaults?.default_time_zone));
  const workspaceWeek = $derived.by(() => {
    const day = ownDefaults?.default_week_start ?? ownDefaults?.installation_defaults?.week_start;
    return leaveTo(day && (WEEK_STARTS as readonly string[]).includes(day) ? t(`app.profile.week_${day}`) : undefined);
  });

  const sourceOf = (key: string, label: (value: string) => string) => {
    const resolved = inForce(settings?.data, `localisation.${key}`);
    const held = key === 'locale' ? languageInForce(resolved, messages.locale) : resolved;
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

  // Whether the AI section has anything to choose: the same read its section draws from.
  $effect(() => untrack(() => aiObjection.open()));
  const hasAiChoice = $derived(aiObjection.position?.shown === true);
  const hasAway = $derived(manifest.value?.modules_on?.includes('SERVICE_DESK') === true);
  const hasAddress = $derived(Boolean(account?.sign_in_name && !account.email && signInRules.isSettled));
  const sections = $derived([
    { id: 'name', code: 'app.profile.name_section' },
    { id: 'language', code: 'app.profile.language_section' },
    { id: 'moments', code: 'app.profile.moments_section' },
    ...(hasAiChoice ? [{ id: 'ai', code: 'app.profile.ai_title' }] : []),
    ...(hasAway ? [{ id: 'away', code: 'app.approvals.delegation_title' }] : []),
    ...(hasAddress ? [{ id: 'address', code: 'app.profile.address_title' }] : []),
  ]);
  const isKnown = (id: string | undefined) => sections.some((each) => each.id === id);
  const current = $derived(isKnown(section) ? (section as string) : 'name');
  const view = $derived<'list' | 'detail'>(isKnown(section) ? 'detail' : 'list');
  const links = $derived<SectionLink[]>(sections.map((each) => ({ id: each.id, label: t(each.code), href: `${BASE}/${each.id}` })));

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
  <SettingsLayout
    head={profileHead('profile')}
    nav={{ label: t('app.settings_page.sections', { name: title }), sections: links, current, onselect: (next: string) => onreplace?.(`${BASE}/${next}`) }}
    {view}
    back={{ label: title, onback: () => onreplace?.('/profile') }}
  >
    <UndeliverableNotice />
    {#if current === 'name' || current === 'language'}
      <!-- Where the tour's `profile` step points: `/profile` opens on the name. The name and the
           language save under one bar, as one form somebody fills in and leaves. -->
      <form data-tour={current === 'name' ? 'profile' : undefined} onsubmit={(event) => { event.preventDefault(); void save(); }}>
        {#if current === 'name'}
          <SettingsSection title={t('app.profile.name_section')} save={saveBar}>
            <div data-setting="name">
              <SettingRow label={t('app.profile.display_name')} help={t('app.profile.display_name_hint')} fieldWidth="md" isRequired>
                {#snippet control(nameId)}
                  <span class="who">
                    <!-- What the others see: the picture, or the initials the name makes. -->
                    <Avatar name={displayName.trim() || account.display_name} src={accounts.pictureOf(account.id)} />
                    <Input label={t('app.profile.display_name')} isLabelHidden aria-labelledby={nameId} bind:value={displayName} error={nameError} maxlength={200} autocomplete="name" isRequired />
                  </span>
                {/snippet}
              </SettingRow>
            </div>
            <div class="picture"><PictureField /></div>
            {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
          </SettingsSection>
        {:else}
          <SettingsSection title={t('app.profile.language_section')} save={saveBar}>
            <div data-setting="language">
              {#if locales.length > 0}
                <SettingRow label={t('app.profile.language')} help={t('app.profile.language_hint')} fieldWidth="md">
                  {#snippet source()}{@render inForceLine(sourceOf('locale', (value) => languageName(value, messages.locale)))}{/snippet}
                  {#snippet control(nameId)}
                    <Select
                      label={t('app.profile.language')}
                      isLabelHidden
                      aria-labelledby={nameId}
                      bind:value={locale}
                      options={withWorkspaceChoice(
                        locales.map((each) => ({ value: each.locale, label: languageName(each.locale, messages.locale) })),
                        workspaceLocale,
                      )}
                    />
                  {/snippet}
                </SettingRow>
              {:else}
                <!-- An installation that declares no locales has none to choose between, and an
                     empty dropdown says that badly. Found by reading a real manifest. -->
                <SettingRow label={t('app.profile.language')} help={t('app.profile.no_locales')}>
                  {#snippet control()}<span></span>{/snippet}
                </SettingRow>
              {/if}
            </div>
            <div data-setting="zone">
              <SettingRow label={t('app.profile.zone')} help={t('app.profile.zone_hint')} fieldWidth="md">
                {#snippet source()}{#if zones.length > 0}{@render inForceLine(sourceOf('time_zone', (value) => value))}{/if}{/snippet}
                {#snippet control(nameId)}
                  <ZonePicker label={t('app.profile.zone')} isLabelHidden aria-labelledby={nameId} freeHint={t('app.profile.zone_free')} inheritLabel={workspaceZone} bind:value={zone} />
                {/snippet}
              </SettingRow>
            </div>
            <div data-setting="week-start">
              <SettingRow label={t('app.profile.week_start')} help={t('app.profile.week_start_hint')} fieldWidth="md">
                {#snippet source()}{@render inForceLine(sourceOf('week_start', (value) => t(`app.profile.week_${value}`)))}{/snippet}
                {#snippet control(nameId)}
                  <Select
                    label={t('app.profile.week_start')}
                    isLabelHidden
                    aria-labelledby={nameId}
                    bind:value={weekStart}
                    options={withWorkspaceChoice(
                      WEEK_STARTS.map((each) => ({ value: each, label: t(`app.profile.week_${each}`) })),
                      workspaceWeek,
                    )}
                  />
                {/snippet}
              </SettingRow>
            </div>
            {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
          </SettingsSection>
        {/if}
      </form>
    {:else if current === 'moments'}
      <!-- The moments: the account's, so here rather than on *On this device* (ADR-0043), written
           at once as every switch outside a form is. -->
      <CelebrationsSwitch />
    {:else if current === 'ai'}
      <!-- Keeping one's own content out of AI: absent where there is nothing to choose (UC-PRV-05
           check 8). -->
      <AiObjectionSection />
    {:else if current === 'away'}
      <!-- Who decides the person's approvals while they are away (UC-SVC-10). -->
      <DelegationSection />
    {:else if current === 'address'}
      <!-- A person without a mail address may add one (UC-ID-20 check 7): drawn exactly where the
           server would take it - a sign-in name and no address, in a workspace whose password is a
           way in. Where it is not, the reason stands in the form's place. -->
      <SettingsSection title={t('app.profile.address_title')} description={signInRules.hasPassword ? t('app.profile.address_intro', { name: account.sign_in_name ?? '' }) : t('app.profile.address_needs_password', { name: account.sign_in_name ?? '' })}>
        {#if signInRules.hasPassword}
          <div class="pad"><AddressForm accountId={account.id} hint={t('app.people.address_hint_own')} /></div>
        {/if}
      </SettingsSection>
    {/if}
  </SettingsLayout>
{/if}

{#snippet saveBar()}
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
{/snippet}

{#snippet inForceLine(held: { source: string; value: string } | undefined)}
  {#if held}
    <span class="in-force" data-source={held.source}>
      <span>{t('app.profile.in_force', { value: held.value })}</span>
      <Badge tone={held.source === 'PERSON' ? 'info' : 'neutral'}>{t(`app.profile.source_${held.source.toLowerCase()}`)}</Badge>
    </span>
  {/if}
{/snippet}

<style>
  form { margin: 0; }

  /* The avatar beside the field, at the field's own line. */
  .who { display: flex; align-items: center; gap: var(--sp-150); }

  .picture, .pad { padding: var(--sp-200) var(--sp-250); border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .pad { border-block-start: 0; }

  .in-force { margin: 0; display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); color: var(--text-secondary); font-size: var(--fs-075); }

  .failure { margin: 0; padding: var(--sp-150) var(--sp-250); color: var(--text-danger); }
</style>
