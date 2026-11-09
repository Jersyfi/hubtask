<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installation's defaults: what every workspace starts with, and whether it may change it
  // (ADR-0070 §2, §5; UC-INS-09).
  //
  // **Read and written here**, because the API is the product and this is one of its three doors:
  // "hubctl und das Dashboard sind zwei Clients davon — dieselbe Regel, die für die Anwendung gilt".
  // An operator who can see a value and has to reach for a terminal to move it is an operator for
  // whom the dashboard is a status page.
  //
  // **Except where the configuration says otherwise**: a file in `enforce` mode is the source, the
  // writing routes refuse, and this screen then offers no controls at all rather than offering a
  // refusal (check 7). `is_enforced_from_file` is what says so, and it is the server's answer.
  //
  // **Every key is drawn, decided or not, with the control of its kind.** The server answers the
  // whole level, each key with its kind, its closed set and its bounds, so the screen keeps no
  // list of its own: a key added tomorrow is drawn by the control of its kind, in the words the
  // catalogue gives it (checks 1 and 2). Nothing is guessed from a current value, which an
  // undecided key does not have.
  //
  // **Every value says what a workspace may still do**, in the words of its kind: make a rule
  // stricter, go higher than a limit, use its own link - or not (check 3). A language, a zone and a
  // week start are defaults and never locked, so they say nothing there (check 4). A workspace sees
  // the same fact from the other side on its own screens.
  //
  // **One area at a time, in tabs** (ux-inventory §E8): sign-in, legal links, language and time,
  // limits, AI - the tab in the address. The editor opens on the whole level, because a save
  // replaces the whole level; switching tabs while editing keeps what was changed, and the one save
  // bar counts every area's changes.
  //
  // **A plan before the save, and the history below** (UC-INS-09 check 9): *Preview the change*
  // asks the server what the save would move, key by key, writing nothing; the history is the
  // journal's saves of the level, each with the keys it moved and the version it made.
  //
  // **The installation's presets below, in multi mode only** (UC-INS-18): written one by one, not
  // with the level, so they are a section of their own beside the editor and not one of its tabs.
  //
  // A settings page (page-system §7.3, §9.4): one section per area at its own address, every value
  // in place in its setting row with where it comes from - the installation or each workspace -
  // and one save bar per section, with *Preview the change* beside it. *History* is a section of
  // its own.

  import { untrack } from 'svelte';

  import { Banner, Input, ReadOnlyNotice, SaveBar, SettingRow, SettingsLayout, SettingsSection, Spinner, Stack } from '@hubtask/design-system/components';
  import type { SectionLink } from '@hubtask/design-system/components';
  import type { InstanceSetting, InstanceSettings, InstanceSettingsPlan } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import InstanceValueField from '../lib/instance/InstanceValueField.svelte';
  import ValueSource from '../lib/workspace/ValueSource.svelte';
  import ViewAsFile from '../lib/settings/ViewAsFile.svelte';
  import { AREAS, changesBetween, choiceCode, documentOf, draftEntry, fieldOf, isBytes, lockCode, nameCode, rowsOf, type Area, type Row } from '../lib/instance/values.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { formatBytes } from '../lib/i18n/bytes.ts';
  import { languageName } from '../lib/i18n/locale.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { localesOf, zoneOptions } from '../lib/data/preferences.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    onnavigate: (path: string) => void;
    /** The section the address names: an area, or `history`. */
    section?: string;
    /** The older address's area (`?tab=`), which replaces itself with the section's own. */
    tab?: string;
    /** Replaces the address when the section changes, so a reload or a link keeps it. */
    onreplace?: (path: string) => void;
  }

  const { onnavigate, section, tab, onreplace }: Props = $props();

  const BASE = '/instance/settings';
  const HISTORY_SECTION = 'history';
  const idOf = (area: Area) => area.replace('_', '-');

  /** What is being edited: a copy of what is in force, refilled after every save. */
  let draft = $state<InstanceSettings | undefined>(undefined);
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let form = $state<HTMLFormElement | undefined>(undefined);
  let isFileOpen = $state(false);

  $effect(() => {
    if (!instance.isElevated) return;
    const stop = untrack(() => instance.openSettings());
    return stop;
  });

  const reading = $derived(instance.settings);
  const settings = $derived(reading.status === 'ready' ? reading.data : undefined);
  const unreadable = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  /** A file that is written at every start makes this level read-only, and says so. */
  const enforced = $derived(settings?.is_enforced_from_file === true);

  // A copy, and a deliberate one: a draft that pointed at the store would change what the source
  // chips say while somebody is typing, and the chips are what they compare against.
  $effect(() => {
    if (settings && !draft) untrack(() => (draft = JSON.parse(JSON.stringify(settings)) as InstanceSettings));
  });
  const draftChanges = $derived(draft && settings ? changesBetween(draft, settings) : 0);

  const rows = $derived(rowsOf(settings));
  /** The areas the server answered keys for, each with its rows, in the page's order. */
  const groups = $derived(
    AREAS.filter((area) => !(area === 'ai' && manifest.isSingleTenant))
      .map((area) => ({ area, rows: rows.filter((row) => row.area === area) }))
      .filter((group) => group.rows.length > 0),
  );

  const isKnown = (id: string | undefined): boolean => id === HISTORY_SECTION || groups.some((group) => idOf(group.area) === id);
  const current = $derived(isKnown(section) ? (section as string) : groups[0] ? idOf(groups[0].area) : HISTORY_SECTION);
  const view = $derived<'list' | 'detail'>(isKnown(section) ? 'detail' : 'list');
  const shown = $derived(groups.find((group) => idOf(group.area) === current));

  // An older address (`?tab=quotas`) lands on its section's own, replacing itself.
  $effect(() => {
    if (section !== undefined || !tab) return;
    const target = AREAS.find((area) => area === tab);
    if (target) untrack(() => onreplace?.(`${BASE}/${idOf(target)}`));
  });

  function select(next: string): void {
    onreplace?.(`${BASE}/${next}`);
  }

  /** How many of an area the installation decided and locked: the section list's state. */
  function stateOf(group: { rows: readonly Row[] }): Pick<SectionLink, 'status' | 'statusTone'> {
    const decided = group.rows.filter((row) => row.setting.set).length;
    const locked = group.rows.filter((row) => row.setting.locked).length;
    if (decided === 0) return { status: t('app.instance.section_each_workspace'), statusTone: 'quiet' };
    return locked > 0
      ? { status: t('app.instance.section_set_locked', { count: String(decided), locked: String(locked) }), statusTone: 'locked' }
      : { status: t('app.instance.section_set', { count: String(decided) }), statusTone: 'set' };
  }

  const links = $derived<SectionLink[]>([
    ...groups.map((group) => ({ id: idOf(group.area), label: t(`app.instance.area_${group.area}`), href: `${BASE}/${idOf(group.area)}`, ...stateOf(group) })),
    { id: HISTORY_SECTION, label: t('app.instance.settings_history_title'), href: `${BASE}/${HISTORY_SECTION}` },
  ]);

  const locales = $derived(localesOf(manifest.value).map((each) => each.locale));

  /** A value as a person reads it: a choice in words, a flag as on or off, a list joined. */
  function said(row: Row): string {
    const { area, key, setting } = row;
    const value = setting.value;
    if (Array.isArray(value)) return value.map((each) => t(choiceCode(area, key, String(each)))).join(', ');
    switch (setting.kind) {
      case 'CHOICE':
        return t(choiceCode(area, key, String(value)));
      case 'FLAG':
        return value === true ? t('app.instance.flag_on') : t('app.instance.flag_off');
      case 'COUNT':
        if (value === 0 && area === 'quotas') return t('app.instance.value_unlimited');
        return isBytes(area, key) && typeof value === 'number' ? formatBytes(value, messages.locale) : String(value);
      case 'LOCALE':
        return languageName(String(value ?? ''), messages.locale);
      default:
        return String(value ?? '');
    }
  }

  /** What a workspace may still do with the key, in words; nothing for a default that is never locked. */
  function mayOf(setting: InstanceSetting): string {
    const code = lockCode(setting, setting.locked);
    return code ? t(code) : '';
  }

  /** How many of the level this installation has actually decided, for the head's figure. */
  const decided = $derived(rows.filter((row) => row.setting.set).length);

  /** Reads one key out of the draft. */
  function currentOf(area: Area, key: string): InstanceSetting | undefined {
    return draftEntry(draft, area, key);
  }

  /** Whether a key in the draft differs from what is in force: the row's bar. */
  function isChanged(row: Row): boolean {
    return JSON.stringify(currentOf(row.area, row.key) ?? null) !== JSON.stringify(row.setting.set ? row.setting : null);
  }

  /**
   * Writes one key back whole, or marks it undecided.
   *
   * `PUT` replaces the level whole, so a key the operator cleared is one the installation stops
   * deciding - and a workspace that had been told the value goes back to deciding for itself.
   */
  function put(area: Area, key: string, declared: InstanceSetting, next: InstanceSetting | undefined): void {
    if (!draft) return;
    const entry: InstanceSetting = next ?? { ...declared, set: false, value: undefined, locked: false };
    draft = { ...draft, [area]: { ...(draft[area] ?? {}), [key]: entry } };
  }

  function discard(): void {
    if (!settings) return;
    draft = JSON.parse(JSON.stringify(settings)) as InstanceSettings;
    failure = undefined;
    plan = undefined;
  }

  /** The section on show as the installation's part of the document (configuration/ui.md A10). */
  const shownValues = $derived(
    (shown?.rows ?? []).map((row) => {
      const entry = currentOf(row.area, row.key);
      return { key: `${row.area}.${row.key}`, value: entry?.value, source: entry?.set ? 'INSTANCE' : 'DEFAULT', lockedBy: entry?.locked ? 'INSTANCE' : undefined };
    }),
  );

  /** The server's answer to *Preview the change*, for the draft it was asked about. */
  let plan = $state<InstanceSettingsPlan | undefined>(undefined);
  async function preview(): Promise<void> {
    if (!draft) return;
    isWorking = true;
    failure = undefined;
    try {
      plan = await instance.planSettings(documentOf(draft));
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('app.something_went_wrong'), fields: new Map(), isServerFault: true };
    } finally {
      isWorking = false;
    }
  }

  const HISTORY = { action: 'instance.settings_changed' } as const;
  $effect(() => {
    if (!instance.isElevated) return;
    return untrack(() => instance.openJournalOf(HISTORY));
  });
  const history = $derived(instance.journalOf(HISTORY));
  const switchesOf = (details: Record<string, unknown> | undefined): string[] =>
    Array.isArray(details?.switches) ? details.switches.filter((each): each is string => typeof each === 'string') : [];

  async function save(): Promise<void> {
    if (!draft) return;
    if (settings && changesBetween(draft, settings) === 0) return;
    isWorking = true;
    failure = undefined;
    try {
      await instance.writeSettings(documentOf(draft));
      draft = undefined;
      plan = undefined;
      toast.show({ text: t('app.instance.settings_saved'), tone: 'success' });
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('app.something_went_wrong'), fields: new Map(), isServerFault: true };
    } finally {
      isWorking = false;
    }
  }

  $effect(() => page.entitle(t('app.instance.settings')));
</script>

<InstanceGate onleave={() => onnavigate('/')}>
  {#snippet children(door)}
    <SettingsLayout
      head={{
        title: t('app.instance.settings'),
        subtitle: rows.length > 0 ? t('app.instance.settings_decided', { decided, total: rows.length }) : undefined,
        breadcrumb: installationTrail({ id: 'screen', label: t('app.instance.settings') }),
        secondary: shown ? [{ label: t('app.settings.view_as_file'), isQuiet: true, onclick: () => (isFileOpen = true) }] : [],
      }}
      nav={{ label: t('app.settings_page.sections', { name: t('app.instance.settings') }), sections: links, current, onselect: select }}
      {view}
      back={{ label: t('app.instance.settings'), onback: () => onreplace?.(BASE) }}
      refused={door ?? (unreadable ? failed : undefined)}
    >
      {#if !settings || !draft}
        <p class="quiet"><Spinner label={t('app.instance.reading')} /><span>{t('app.instance.reading')}</span></p>
      {:else if current === HISTORY_SECTION}
        <SettingsSection title={t('app.instance.settings_history_title')}>
          {#if history.length === 0}
            <p class="quiet-line">{t('app.instance.settings_history_none')}</p>
          {:else}
            <ul class="changes">
              {#each history as entry (entry.id)}
                <li data-history-entry>
                  {formatDateTime(entry.occurred_at, messages.locale)} · {entry.actor_label ?? t('app.instance.settings_history_file')}
                  {#if typeof entry.details?.version === 'number'} · {t('app.instance.settings_history_version', { version: String(entry.details.version) })}{/if}
                  <span class="quiet-line">{switchesOf(entry.details).join(', ')}</span>
                </li>
              {/each}
            </ul>
          {/if}
        </SettingsSection>
      {:else if shown}
        {@const group = shown}
        {@const editing = draft}
        <form bind:this={form} onsubmit={(event) => { event.preventDefault(); void save(); }}>
          <SettingsSection
            title={t(`app.instance.area_${group.area}`)}
            description={group.area === 'localisation' ? t('app.instance.locked_never_localisation') : t('app.instance.settings_intro')}
            status={stateOf(group).status}
          >
            {#if enforced}
              <!-- The file is the source, so nothing here is writable and no control is drawn. A
                   form that offered a save which the server refuses would be a form that lies. -->
              <ReadOnlyNotice text={`${t('app.instance.source_file_enforced')} ${settings.source ?? ''}`} />
            {/if}
            {#if failure && failure.fields.size === 0}<FailureNote problem={failure} />{/if}
            {#each group.rows as row (row.key)}
              <SettingRow label={t(nameCode(row.area, row.key))} help={mayOf(row.setting) || undefined} fieldWidth="md" isChanged={!enforced && isChanged(row)}>
                {#snippet source()}
                  <ValueSource source={row.setting.set ? 'INSTANCE' : 'DEFAULT'} lock={row.setting.locked ? 'INSTANCE' : null} value={row.setting.set ? said(row) : t('app.instance.value_undecided')} />
                {/snippet}
                {#snippet control(nameId)}
                  {#if enforced}
                    <span class="value">{row.setting.set ? said(row) : t('app.instance.value_undecided')}</span>
                  {:else}
                    <InstanceValueField
                      area={row.area}
                      key={row.key}
                      declared={row.setting}
                      current={currentOf(row.area, row.key)}
                      {locales}
                      zones={zoneOptions(String(editing.localisation?.time_zone?.value ?? ''))}
                      error={failure?.fields.get(fieldOf(row.area, row.key))}
                      {nameId}
                      onchange={(next) => put(row.area, row.key, row.setting, next)}
                    />
                  {/if}
                {/snippet}
              </SettingRow>
            {/each}
            {#if group.area === 'sign_in'}
              <!-- The blocklist is the sign-in rules' too: the installation's alone. -->
              <SettingRow label={t('app.instance.blocklist_label')} help={t('app.instance.blocklist_hint')} fieldWidth="lg" isChanged={(editing.blocklist_file ?? '') !== (settings.blocklist_file ?? '')}>
                {#snippet control(nameId)}
                  {#if enforced}
                    <span class="value">{settings.blocklist_file ?? ''}</span>
                  {:else}
                    <Input
                      label={t('app.instance.blocklist_label')}
                      isLabelHidden
                      aria-labelledby={nameId}
                      value={editing.blocklist_file ?? ''}
                      oninput={(event) => {
                        if (!draft) return;
                        draft = { ...draft, blocklist_file: (event.currentTarget as HTMLInputElement).value };
                      }}
                      autocomplete="off"
                      spellcheck={false}
                    />
                  {/if}
                {/snippet}
              </SettingRow>
            {/if}
            {#if plan}
              <Stack gap="100">
                <h3 class="subsection">{t('app.instance.settings_plan_title')}</h3>
                {#if plan.changes.length === 0}
                  <p class="quiet-line">{t('app.instance.settings_plan_none')}</p>
                {:else}
                  <ul class="changes">
                    {#each plan.changes as change (change.key)}
                      <li data-plan-change><code>{change.key}</code>
                        {t('app.instance.settings_plan_change', {
                          from: change.from === '' ? t('app.instance.value_undecided') : change.from,
                          to: change.to === '' ? t('app.instance.value_undecided') : change.to,
                        })}{change.from_locked !== change.to_locked ? ` · ${change.to_locked ? t('app.instance.settings_plan_locks') : t('app.instance.settings_plan_unlocks')}` : ''}</li>
                    {/each}
                  </ul>
                {/if}
                {#if plan.applied_through === 'FILE'}
                  <p class="quiet-line">{t('app.instance.settings_plan_file', { path: plan.source ?? '' })}</p>
                {/if}
              </Stack>
            {/if}
            {#snippet save()}
              {#if !enforced && draftChanges > 0}
                <SaveBar
                  label={t('app.save_bar.label')}
                  changesLabel={t('app.save_bar.changes', { count: String(draftChanges) })}
                  discardLabel={t('app.save_bar.discard')}
                  saveLabel={t('app.instance.settings_save')}
                  isSaving={isWorking}
                  savingLabel={t('app.instance.working')}
                  planLabel={t('app.instance.settings_plan_preview')}
                  onPlan={() => void preview()}
                  onDiscard={discard}
                  onSave={() => form?.requestSubmit()}
                />
              {/if}
            {/snippet}
          </SettingsSection>
        </form>
      {:else}
        <Banner tone="info">{t('app.instance.settings_none')}</Banner>
      {/if}
    </SettingsLayout>
  {/snippet}
</InstanceGate>

<ViewAsFile values={shownValues} place="INSTANCE" bind:isOpen={isFileOpen} isTriggerHidden />

{#snippet failed()}
  {#if unreadable}<FailureNote problem={unreadable} />{/if}
{/snippet}

<style>
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .quiet-line { margin: 0; max-inline-size: 60ch; color: var(--text-secondary); font-size: var(--fs-100); }
  .value { overflow-wrap: anywhere; }
  .subsection { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .changes { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); overflow-wrap: anywhere; }
  form { margin: 0; }
</style>
