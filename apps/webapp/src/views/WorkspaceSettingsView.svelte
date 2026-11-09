<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // How the workspace is set up: its name, what its members fall back to, what people without an
  // account may open, and what a new hub starts from. How they sign in is the sign-in screen's
  // alone (UC-ID-12 check 1).
  //
  // **One section per address, one save each** (page-system §7.3, PS-9, PS-10): *Name and look*,
  // *Language and time*, *Help and support* and *New hubs*, each at
  // `/administration/workspace/<section>`. A section's save bar counts and sends its own values;
  // what moved in another section waits there, which is why a move between sections replaces the
  // address instead of passing the leave guard. *Help and support* holds switches that act at once
  // and so has no bar.
  //
  // **The three defaults are stated as what they are.** The locale and the zone here are not "the
  // workspace's language" — they are the third link of `i18n-l10n.md` §2's chain: what a member
  // who has set nothing falls back to. Somebody who reads them as "everybody's language" changes
  // them and is surprised that most people see nothing different.
  //
  // **The installation's default is said beside each of them** (UC-INS-09 check 5): what a new
  // workspace started with, and for the week start what applies while this workspace decides
  // none. Only where the installation decided one - a private installation is not a column here.
  //
  // **New hubs** (UC-WRK-30 check 4): the preset a hub starts from when whoever creates it chooses
  // none, and the settings locked for every hub and collection. The default is not the
  // installation's - it can be withdrawn without the workspace knowing, and a default naming nothing
  // would break every new hub, so the server refuses it and it is not offered. A lock is a whole set:
  // `PATCH /tenant/structure` takes every workspace lock at once, so the locks of one field of one
  // level - set on the *Structure* panel - travel back as they are and are shown, not offered. The
  // installation's and the plan's locks are not the workspace's to lift.
  //
  // The address is not here: it has a screen of its own, *Address*, with who may change it.

  import { tick, untrack } from 'svelte';

  import { CheckboxGroup, Input, SaveBar, Select, SettingRow, SettingsLayout, SettingsSection, Spinner } from '@hubtask/design-system/components';
  import type { SectionLink } from '@hubtask/design-system/components';
  import ZonePicker from '../lib/forms/ZonePicker.svelte';
  import { TransportError } from '@hubtask/sync-engine';

  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { presetChoice, presetName, presets, presetValue } from '../lib/data/presets.svelte.ts';
  import { isModuleOn } from '../lib/frame/moduleroute.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { WEEK_STARTS, localesOf } from '../lib/data/preferences.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { languageName } from '../lib/i18n/locale.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { enterSubmits } from '../lib/forms/enter.ts';
  import { check, focusFirst, isComplete, type Missing } from '../lib/forms/missing.ts';
  import { leaveGuard } from '../lib/frame/leave.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import ValueSource from '../lib/workspace/ValueSource.svelte';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';
  import WriteOnly from '../lib/workspace/WriteOnly.svelte';
  import { SETTINGS } from '../lib/workspace/structureDraft.ts';
  import PublicLinksSwitch from '../lib/pages/PublicLinksSwitch.svelte';
  import PublicFormsSwitch from '../lib/servicedesk/PublicFormsSwitch.svelte';
  import PublicRoadmapsSwitch from '../lib/goals/PublicRoadmapsSwitch.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import ViewSectionsAsFile from '../lib/settings/ViewSectionsAsFile.svelte';
  import { pagePart } from '../lib/settings/asFile.ts';
  import { viewAsFileAction } from '../lib/settings/viewasfile.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    /** The section the address names (`/administration/workspace/:section`); none is the list. */
    section?: string;
    /** Replaces the address: a move between sections, so the unsaved values of another stay. */
    onreplace?: (path: string) => void;
  }
  const { section, onreplace }: Props = $props();

  const BASE = '/administration/workspace';
  /** Which values each section's save bar counts and sends. */
  const SCOPES: Readonly<Record<string, readonly string[]>> = {
    name: ['display_name'],
    language: ['default_locale', 'default_time_zone', 'default_week_start'],
  };

  let displayName = $state('');
  let locale = $state('');
  let zone = $state('');
  let weekStart = $state('');
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let form = $state<HTMLFormElement | undefined>(undefined);
  let missing = $state<Missing>({});
  let isFileOpen = $state(false);
  /** Filled from the read once, so that typing is not overwritten by the re-read that follows a save. */
  let filled = false;

  $effect(() => untrack(() => workspace.open()));
  $effect(() => untrack(() => presets.open()));

  const reading = $derived(workspace.state);
  const current = $derived(workspace.workspace);
  const structure = $derived(current?.structure);
  const locales = $derived(localesOf(manifest.value));

  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  $effect(() => {
    const held = current;
    if (!held || filled) return;
    filled = true;
    displayName = held.display_name;
    locale = held.default_locale;
    zone = held.default_time_zone;
    weekStart = held.default_week_start ?? '';
  });

  // The switches of *Help and support*, each its product's: read while the module is off, its
  // policy is refused; and each drawn only for whoever manages the members.
  const modulesOn = $derived(manifest.value?.modules_on);
  const hasHelp = $derived(
    holdsHere('MANAGE_MEMBERS') === true &&
      (isModuleOn('KNOWLEDGE', modulesOn) || isModuleOn('SERVICE_DESK', modulesOn) || isModuleOn('GOALS', modulesOn)),
  );

  const SECTIONS = [
    { id: 'name', code: 'app.workspace.section_name' },
    { id: 'language', code: 'app.workspace.section_language' },
    { id: 'help', code: 'app.workspace.section_help' },
    { id: 'new-hubs', code: 'app.workspace.section_new_hubs' },
  ] as const;
  const sections = $derived(SECTIONS.filter((each) => each.id !== 'help' || hasHelp));
  const isKnown = (id: string | undefined): boolean => id !== undefined && sections.some((each) => each.id === id);
  const shown = $derived(isKnown(section) ? (section as string) : 'name');
  /** At the screen's own address the list - with the first section beside it where there is room. */
  const view = $derived<'list' | 'detail'>(isKnown(section) ? 'detail' : 'list');

  const above = $derived(current?.installation_defaults ?? {});
  /** The day the week starts on while this workspace decides none, in words. */
  const weekStartAbove = $derived(
    above.week_start
      ? t('app.workspace.week_start_installation', { day: t(`app.profile.week_${above.week_start}`) })
      : t('app.workspace.week_start_language'),
  );

  /** What moved, by the contract's names. Merge-patch means an absent key changes nothing. */
  function movement(): Record<string, string> {
    const held = current;
    if (!held) return {};
    return {
      ...(displayName.trim() !== held.display_name ? { display_name: displayName.trim() } : {}),
      ...(locale !== held.default_locale ? { default_locale: locale } : {}),
      ...(zone !== held.default_time_zone ? { default_time_zone: zone } : {}),
      ...(weekStart !== (held.default_week_start ?? '') ? { default_week_start: weekStart } : {}),
    };
  }

  /** What moved in one section: what its save bar counts and sends. */
  function moved(scope: string): Record<string, string> {
    const keys = SCOPES[scope] ?? [];
    return Object.fromEntries(Object.entries(movement()).filter(([key]) => keys.includes(key)));
  }

  const changedCount = $derived(Object.keys(moved(shown)).length);
  const isChanged = (key: string): boolean => key in movement();

  /** One section's values back as the workspace holds them; another section's stay. */
  function discard(): void {
    const held = current;
    if (!held) return;
    failure = undefined;
    missing = {};
    if (shown === 'name') displayName = held.display_name;
    if (shown === 'language') {
      locale = held.default_locale;
      zone = held.default_time_zone;
      weekStart = held.default_week_start ?? '';
    }
  }

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const body = moved(shown);
    // A save with nothing in it would be a request that writes nothing and a version bump that
    // means nothing to whoever reads the trail.
    if (Object.keys(body).length === 0) return;
    if (shown === 'name') {
      missing = check([{ kind: 'field', key: 'name', code: 'app.workspace.name_needed', isFilled: displayName.trim() !== '' }]);
      if (!isComplete(missing)) {
        void tick().then(() => focusFirst(form));
        return;
      }
    }

    isWorking = true;
    failure = undefined;
    try {
      await workspace.change(body);
      toast.show({ text: t('app.workspace.saved'), tone: 'success' });
    } catch (cause) {
      failure = problemOf(cause);
    } finally {
      isWorking = false;
    }
  }

  function problemOf(cause: unknown): ReturnType<typeof renderProblem> {
    return cause instanceof TransportError
      ? renderProblem(cause, messages)
      : { message: messages.t('app.something_went_wrong'), fields: new Map(), isServerFault: true };
  }

  // New hubs: the default preset and the locks, each untouched while it follows the read.
  /** Hubtask's key or a workspace preset's id, or empty for none, as the workspace holds it. */
  const defaultValue = $derived(structure?.default_preset ?? structure?.default_preset_id ?? '');
  const defaultOptions = $derived([
    { value: '', label: t('app.presets.default_none') },
    ...presets.all.filter((preset) => preset.origin === 'HUBTASK' || preset.origin === 'WORKSPACE').map((preset) => ({ value: presetValue(preset), label: presetName(preset) })),
  ]);
  /** The workspace's own locks: on a whole setting, and on one field of one level. */
  const workspaceLocks = $derived(
    Object.entries(structure?.locks ?? {}).filter(([, lock]) => lock.origin === 'WORKSPACE').map(([key]) => key),
  );
  const lockedSettings = $derived(workspaceLocks.filter((key) => !key.includes('.')));
  const fieldLocks = $derived(workspaceLocks.filter((key) => key.includes('.')));
  /** Locks the workspace cannot lift: the installation's and the plan's. */
  const lockedAbove = $derived(
    Object.entries(structure?.locks ?? {}).filter(([, lock]) => lock.origin !== 'WORKSPACE').map(([key, lock]) => ({ key, origin: lock.origin })),
  );
  const lockOptions = $derived(
    SETTINGS.map((setting) => {
      const lock = lockedAbove.find((each) => each.key === setting);
      return {
        value: setting,
        label: t(`app.structure.setting.${setting}`),
        ...(lock ? { disabledReason: t('app.presets.locked_above', { origin: t('app.structure.locked_origin', { origin: lock.origin }) }) } : {}),
      };
    }),
  );

  let chosenDefault = $state<string | undefined>(undefined);
  let chosenLocks = $state<readonly string[] | undefined>(undefined);
  let isSavingHubs = $state(false);
  let hubsFailure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  const sameSet = (a: readonly string[], b: readonly string[]): boolean => a.length === b.length && a.every((each) => b.includes(each));
  const isDefaultMoved = $derived(chosenDefault !== undefined && chosenDefault !== defaultValue);
  const areLocksMoved = $derived(chosenLocks !== undefined && !sameSet(chosenLocks, lockedSettings));
  const hubsChanged = $derived((isDefaultMoved ? 1 : 0) + (areLocksMoved ? 1 : 0));

  function discardHubs(): void {
    chosenDefault = undefined;
    chosenLocks = undefined;
    hubsFailure = undefined;
  }

  async function saveHubs(): Promise<void> {
    if (hubsChanged === 0) return;
    const choice = isDefaultMoved ? presetChoice(chosenDefault ?? '') : undefined;
    const body = {
      ...(choice ? (choice.preset_id ? { default_preset_id: choice.preset_id } : { default_preset: choice.preset ?? null }) : {}),
      ...(areLocksMoved ? { locks: [...(chosenLocks ?? []), ...fieldLocks] } : {}),
    };
    isSavingHubs = true;
    hubsFailure = undefined;
    try {
      await workspace.changeStructure(body);
      const text = isDefaultMoved && areLocksMoved ? 'app.workspace.saved' : isDefaultMoved ? 'app.presets.default_saved' : 'app.presets.locks_saved';
      discardHubs();
      toast.show({ text: t(text), tone: 'success' });
    } catch (cause) {
      hubsFailure = problemOf(cause);
    } finally {
      isSavingHubs = false;
    }
  }

  /** Every section's unsaved values: what the leave guard holds, as a move between sections keeps them. */
  const unsaved = $derived(Object.keys(movement()).length + hubsChanged);
  $effect(() => leaveGuard.hold(() => unsaved));

  /** A section's state in the list, from what is read: never a guess before the read answers. */
  function statusOf(id: string): Pick<SectionLink, 'status' | 'statusTone'> {
    const held = current;
    if (!held) return {};
    if (id === 'language') {
      const set = [
        Boolean(above.locale) && held.default_locale !== above.locale,
        Boolean(above.time_zone) && held.default_time_zone !== above.time_zone,
        Boolean(held.default_week_start),
      ].filter(Boolean).length;
      return set > 0 ? { status: t('app.workspace.status_set', { count: set }), statusTone: 'set' } : { status: t('app.workspace.status_default') };
    }
    if (id === 'new-hubs') {
      const set = (defaultValue !== '' ? 1 : 0) + lockedSettings.length;
      return set > 0 ? { status: t('app.workspace.status_set', { count: set }), statusTone: 'set' } : { status: t('app.workspace.status_default') };
    }
    return {};
  }

  const links = $derived<SectionLink[]>(sections.map((each) => ({ id: each.id, label: t(each.code), href: `${BASE}/${each.id}`, ...statusOf(each.id) })));
  const title = $derived(t('app.workspace.title'));
  const fileAction = $derived(viewAsFileAction(() => (isFileOpen = true)));

  function select(next: string): void {
    onreplace?.(`${BASE}/${next}`);
  }

  $effect(() => page.entitle(title));
</script>

{#snippet saveBar()}
  <WriteOnly>
    <SaveBar
      label={t('app.save_bar.label')}
      changesLabel={changedCount > 0 ? t('app.save_bar.changes', { count: String(changedCount) }) : undefined}
      discardLabel={t('app.save_bar.discard')}
      saveLabel={t('app.workspace.save')}
      isSaving={isWorking}
      savingLabel={t('app.workspace.saving')}
      onDiscard={discard}
      onSave={() => form?.requestSubmit()}
    />
  </WriteOnly>
{/snippet}

{#snippet hubsSaveBar()}
  <WriteOnly>
    <SaveBar
      label={t('app.save_bar.label')}
      changesLabel={hubsChanged > 0 ? t('app.save_bar.changes', { count: String(hubsChanged) }) : undefined}
      discardLabel={t('app.save_bar.discard')}
      saveLabel={t('app.workspace.save')}
      isSaving={isSavingHubs}
      savingLabel={t('app.workspace.saving')}
      onDiscard={discardHubs}
      onSave={() => void saveHubs()}
    />
  </WriteOnly>
{/snippet}

<SettingsLayout
  head={{
    title,
    breadcrumb: {
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'workspace', label: title },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    },
    secondary: fileAction ? [fileAction] : [],
  }}
  nav={{ label: t('app.settings_page.sections', { name: title }), sections: links, current: shown, onselect: select }}
  {view}
  back={{ label: title, onback: () => onreplace?.(BASE) }}
>
  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="quiet">
      <Spinner label={t('app.workspace.reading')} />
      <span>{t('app.workspace.reading')}</span>
    </p>
  {:else if refusal}
    <FailureNote problem={refusal} />
  {:else if current}
    {#if shown === 'name' || shown === 'language'}
      {#if failure}<FailureNote problem={failure} />{/if}
      <WorkspaceGate permission="STRUCTURE">
        <form bind:this={form} onsubmit={save} use:enterSubmits>
          {#if shown === 'name'}
            <SettingsSection title={t('app.workspace.section_name')} save={saveBar}>
              <SettingRow label={t('app.workspace.name')} help={t('app.workspace.name_hint')} fieldWidth="md" isRequired isChanged={isChanged('display_name')}>
                {#snippet control(nameId)}
                  <Input
                    label={t('app.workspace.name')}
                    isLabelHidden
                    aria-labelledby={nameId}
                    bind:value={displayName}
                    error={(missing.name && !displayName.trim() ? t(missing.name) : undefined) ?? failure?.fields.get('display_name')}
                    isRequired
                  />
                {/snippet}
              </SettingRow>
            </SettingsSection>
          {:else}
            <SettingsSection title={t('app.workspace.section_language')} description={t('app.workspace.section_language_lead')} status={statusOf('language').status} save={saveBar}>
              <div class="row">
                <SettingRow
                  label={t('app.workspace.locale')}
                  help={above.locale
                    ? `${t('app.workspace.locale_hint')} ${t('app.workspace.installation_default', { value: languageName(above.locale, messages.locale) })}`
                    : t('app.workspace.locale_hint')}
                  fieldWidth="md"
                  isChanged={isChanged('default_locale')}
                >
                  {#snippet control(nameId)}
                    {#if locales.length > 0}
                      <Select
                        label={t('app.workspace.locale')}
                        isLabelHidden
                        aria-labelledby={nameId}
                        bind:value={locale}
                        options={locales.map((each) => ({ value: each.locale, label: languageName(each.locale, messages.locale) }))}
                      />
                    {:else}
                      <!-- An installation that declares no locales has none to choose between, and
                           the server still refuses one it cannot resolve. -->
                      <Input
                        label={t('app.workspace.locale')}
                        isLabelHidden
                        aria-labelledby={nameId}
                        hint={t('app.workspace.locale_free')}
                        bind:value={locale}
                        error={failure?.fields.get('default_locale')}
                      />
                    {/if}
                  {/snippet}
                </SettingRow>
              </div>
              <div class="row">
                <SettingRow
                  label={t('app.workspace.zone')}
                  help={above.time_zone
                    ? `${t('app.workspace.zone_hint')} ${t('app.workspace.installation_default', { value: above.time_zone })}`
                    : t('app.workspace.zone_hint')}
                  fieldWidth="md"
                  isChanged={isChanged('default_time_zone')}
                >
                  {#snippet control(nameId)}
                    <ZonePicker
                      label={t('app.workspace.zone')}
                      isLabelHidden
                      aria-labelledby={nameId}
                      freeHint={t('app.workspace.zone_free')}
                      bind:value={zone}
                      error={failure?.fields.get('default_time_zone')}
                    />
                  {/snippet}
                </SettingRow>
              </div>
              <div class="row">
                <SettingRow label={t('app.workspace.week_start')} help={t('app.workspace.week_start_hint')} fieldWidth="md" isChanged={isChanged('default_week_start')}>
                  {#snippet source()}
                    {#if weekStart === '' && above.week_start}
                      <!-- Following the installation: the chip says whose day it is (UC-WRK-32
                           check 5). A week that follows each person's language is this
                           workspace's own choice. -->
                      <ValueSource source="INSTANCE" value={t(`app.profile.week_${above.week_start}`)} />
                    {/if}
                  {/snippet}
                  {#snippet control(nameId)}
                    <Select
                      label={t('app.workspace.week_start')}
                      isLabelHidden
                      aria-labelledby={nameId}
                      bind:value={weekStart}
                      options={[
                        { value: '', label: weekStartAbove },
                        ...WEEK_STARTS.map((each) => ({ value: each, label: t(`app.profile.week_${each}`) })),
                      ]}
                      error={failure?.fields.get('/default_week_start')}
                    />
                  {/snippet}
                </SettingRow>
              </div>
              <!-- No second-factor switch here: who needs one is the sign-in rule's, set on the
                   sign-in screen with its step-up, its lock and its direction. A switch here was a
                   second place for the same rule, stored apart from it, and the two disagreed
                   (UC-ID-12 check 1). -->
            </SettingsSection>
          {/if}
        </form>
      </WorkspaceGate>
    {:else if shown === 'help'}
      <!-- Switches that act at once: no save bar (PS-10). -->
      <SettingsSection title={t('app.workspace.section_help')} description={t('app.workspace.section_help_lead')}>
        {#if isModuleOn('KNOWLEDGE', modulesOn)}<div class="row"><PublicLinksSwitch /></div>{/if}
        {#if isModuleOn('SERVICE_DESK', modulesOn)}<div class="row"><PublicFormsSwitch /></div>{/if}
        {#if isModuleOn('GOALS', modulesOn)}<div class="row"><PublicRoadmapsSwitch /></div>{/if}
      </SettingsSection>
    {:else if shown === 'new-hubs' && structure}
      {#if hubsFailure}<FailureNote problem={hubsFailure} />{/if}
      <WorkspaceGate permission="STRUCTURE">
        <SettingsSection title={t('app.workspace.section_new_hubs')} description={t('app.workspace.section_new_hubs_lead')} status={statusOf('new-hubs').status} save={hubsSaveBar}>
          <div class="row" data-presets-default>
            <SettingRow label={t('app.presets.default_label')} help={t('app.presets.default_hint')} fieldWidth="md" isChanged={isDefaultMoved}>
              {#snippet control(nameId)}
                <Select
                  label={t('app.presets.default_label')}
                  isLabelHidden
                  aria-labelledby={nameId}
                  options={defaultOptions}
                  bind:value={() => chosenDefault ?? defaultValue, (value) => (chosenDefault = value)}
                />
              {/snippet}
            </SettingRow>
          </div>
          <div class="row" data-presets-locks>
            <SettingRow label={t('app.presets.locks_title')} help={t('app.presets.locks_hint')} fieldWidth="lg" isChanged={areLocksMoved}>
              {#snippet control()}
                <CheckboxGroup
                  label={t('app.presets.locks_label')}
                  options={lockOptions}
                  selected={chosenLocks ?? lockedSettings}
                  onChange={(selected) => (chosenLocks = selected)}
                />
                {#if fieldLocks.length > 0}
                  <p class="quiet" data-field-locks>{t('app.presets.field_locks', { count: String(fieldLocks.length) })}</p>
                {/if}
              {/snippet}
            </SettingRow>
          </div>
        </SettingsSection>
      </WorkspaceGate>
    {/if}
  {/if}
</SettingsLayout>

<ViewSectionsAsFile {...pagePart(BASE)} bind:isOpen={isFileOpen} isTriggerHidden />

<style>
  form { margin: 0; }

  .quiet {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }

  /* Each row is wrapped so a test and a reader find its control and its parts together; the
     hairline the design system draws between two rows is drawn here between two wrapped ones,
     since its own rule sees siblings. */
  .row + .row :global(.setting-row > .setting-line:first-child) { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
</style>
