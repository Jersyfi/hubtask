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

  import { untrack } from 'svelte';

  import { Banner, Button, Input, PageHeader, Spinner, Stack, Table } from '@hubtask/design-system/components';
  import type { InstanceSetting, InstanceSettings } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import InstanceValueField from '../lib/instance/InstanceValueField.svelte';
  import { AREAS, choiceCode, documentOf, draftEntry, fieldOf, isBytes, lockCode, nameCode, rowsOf, type Area, type Row } from '../lib/instance/values.ts';
  import { formatBytes } from '../lib/i18n/bytes.ts';
  import { languageName } from '../lib/i18n/locale.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { localesOf, zoneOptions } from '../lib/data/preferences.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  interface Props {
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  /** The document being edited, or nothing while it is only being read. */
  let draft = $state<InstanceSettings | undefined>(undefined);
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let saved = $state(false);

  $effect(() => {
    const stop = untrack(() => instance.openSettings());
    return stop;
  });

  const reading = $derived(instance.settings);
  const settings = $derived(reading.status === 'ready' ? reading.data : undefined);
  const unreadable = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );
  /** A file that is written at every start makes this level read-only, and says so. */
  const enforced = $derived(settings?.is_enforced_from_file === true);

  const rows = $derived(rowsOf(settings));
  /** The areas the server answered keys for, each with its rows, in the screen's order. */
  // A private installation offers no model of its own to its one workspace, so the AI area - which
  // is about the offered model - is not drawn there (UC-AI-06 check 8). Its values are kept as
  // they are and sent back unchanged.
  const groups = $derived(
    AREAS.filter((area) => !(area === 'ai' && manifest.isSingleTenant))
      .map((area) => ({ area, rows: rows.filter((row) => row.area === area) }))
      .filter((group) => group.rows.length > 0),
  );

  const locales = $derived(localesOf(manifest.value).map((each) => each.locale));

  const columns = $derived([
    { id: 'setting', label: t('app.instance.column_setting') },
    { id: 'value', label: t('app.instance.column_value') },
    { id: 'lock', label: t('app.instance.workspaces_may') },
  ]);

  /** A value as the table says it: a choice in words, a flag as on or off, a list joined. */
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
        // Zero is a limit's "unlimited", which is a decision and not a number anybody counts to.
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

  /** How many of the level this installation has actually decided, for the line above it. */
  const decided = $derived(rows.filter((row) => row.setting.set).length);

  /**
   * Opens the editor on what is in force.
   *
   * A copy, and a deliberate one: a draft that pointed at the store would change what the table
   * beside it draws while somebody is typing, and the table is what they are comparing against.
   */
  function edit(): void {
    if (!settings) return;
    draft = JSON.parse(JSON.stringify(settings)) as InstanceSettings;
    failure = undefined;
    saved = false;
  }

  /** Reads one key out of the draft. */
  function current(area: Area, key: string): InstanceSetting | undefined {
    return draftEntry(draft, area, key);
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

  async function save(): Promise<void> {
    if (!draft) return;
    isWorking = true;
    failure = undefined;
    saved = false;
    try {
      await instance.writeSettings(documentOf(draft));
      draft = undefined;
      saved = true;
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
    } finally {
      isWorking = false;
    }
  }

  $effect(() => page.entitle(t('app.instance.settings')));
</script>

<Stack gap="300">
  <PageHeader title={t('app.instance.settings')} isTitleInBar={viewport.isCompact} />

  <InstanceGate onleave={() => onnavigate('/')}>
    <Stack gap="200">
      <p class="prose">{t('app.instance.settings_intro')}</p>

      {#if failure}
        <Banner tone="danger" title={failure.message}>
          {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
        </Banner>
      {:else if saved}
        <Banner tone="success">{t('app.instance.settings_saved')}</Banner>
      {/if}

      {#if reading.status === 'loading' || reading.status === 'idle'}
        <p class="quiet">
          <Spinner label={t('app.instance.reading')} />
          <span>{t('app.instance.reading')}</span>
        </p>
      {:else if unreadable}
        <Banner tone="danger" title={unreadable.message}>
          {#if unreadable.reference}{t('app.error_reference', { request_id: unreadable.reference })}{/if}
        </Banner>
      {:else}
        {#if enforced}
          <!-- The file is the source, so nothing here is writable and no control is drawn. A form
               that offered a save which the server refuses would be a form that lies. -->
          <Banner tone="info" title={t('app.instance.source_file_enforced')}>
            {settings?.source ?? ''}
          </Banner>
        {/if}
        <p class="quiet-line">{t('app.instance.settings_decided', { decided, total: rows.length })}</p>
        {#if settings?.blocklist_file}
          <p class="quiet-line">{t('app.instance.blocklist_file', { path: settings.blocklist_file })}</p>
        {/if}

        {#if draft}
          {@const editing = draft}
          <form
            class="panel"
            onsubmit={(event) => {
              event.preventDefault();
              void save();
            }}
          >
            <Stack gap="300">
              <Stack gap="100">
                <h2 class="section">{t('app.instance.settings_edit_title')}</h2>
                <p class="quiet-line">{t('app.instance.settings_edit_hint')}</p>
              </Stack>

              {#each groups as group (group.area)}
                <section class="group" aria-labelledby={`area-${group.area}`}>
                  <Stack gap="150">
                    <h3 id={`area-${group.area}`} class="area">{t(`app.instance.area_${group.area}`)}</h3>
                    {#if group.area === 'localisation'}
                      <p class="quiet-line">{t('app.instance.locked_never_localisation')}</p>
                    {/if}
                    {#each group.rows as row (row.key)}
                      <InstanceValueField
                        area={row.area}
                        key={row.key}
                        declared={row.setting}
                        current={current(row.area, row.key)}
                        {locales}
                        zones={zoneOptions(String(editing.localisation?.time_zone?.value ?? ''))}
                        error={failure?.fields.get(fieldOf(row.area, row.key))}
                        onchange={(next) => put(row.area, row.key, row.setting, next)}
                      />
                    {/each}
                  </Stack>
                </section>
              {/each}

              <Input
                label={t('app.instance.blocklist_label')}
                hint={t('app.instance.blocklist_hint')}
                value={editing.blocklist_file ?? ''}
                oninput={(event) => {
                  if (!draft) return;
                  draft = { ...draft, blocklist_file: (event.currentTarget as HTMLInputElement).value };
                }}
                autocomplete="off"
                spellcheck={false}
              />

              <div class="row">
                <Button type="submit" tone="primary" isBusy={isWorking} busyLabel={t('app.instance.working')}>
                  {t('app.instance.settings_save')}
                </Button>
                <Button tone="subtle" onclick={() => (draft = undefined)}>
                  {t('app.instance.cancel')}
                </Button>
              </div>
            </Stack>
          </form>
        {:else if rows.length === 0}
          <Banner tone="info">{t('app.instance.settings_none')}</Banner>
        {:else}
          {#each groups as group (group.area)}
            <section aria-labelledby={`read-${group.area}`}>
              <Stack gap="100">
                <h2 id={`read-${group.area}`} class="section">{t(`app.instance.area_${group.area}`)}</h2>
                <Table label={t(`app.instance.area_${group.area}`)} isLabelHidden {columns}>
                  {#each group.rows as row (row.key)}
                    <tr>
                      <td>{t(nameCode(row.area, row.key))}</td>
                      <td class={row.setting.kind === 'LINK' ? 'link' : ''}>
                        {#if row.setting.set}
                          {said(row)}
                        {:else}
                          <!-- Not a blank cell: "this installation decided nothing here" is a fact
                               worth a sentence, and the workspace is who decides it instead. -->
                          <span class="undecided">{t('app.instance.value_undecided')}</span>
                        {/if}
                      </td>
                      <td>{mayOf(row.setting)}</td>
                    </tr>
                  {/each}
                </Table>
              </Stack>
            </section>
          {/each}
        {/if}

        {#if !draft && !enforced}
          <div>
            <Button tone="primary" onclick={edit}>{t('app.instance.settings_edit')}</Button>
          </div>
        {/if}
      {/if}
    </Stack>
  </InstanceGate>
</Stack>

<style>
  .prose { margin: 0; max-inline-size: 60ch; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .quiet-line { margin: 0; max-inline-size: 60ch; color: var(--text-secondary); font-size: var(--fs-100); }
  .link { overflow-wrap: anywhere; }
  .undecided { color: var(--text-secondary); font-size: var(--fs-100); }
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  .area { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }

  /* A form is neither prose nor a table (ADR-0065 decision 2): the fields carry a measure of their
     own while the tables take the width. */
  form { margin: 0; max-inline-size: 80ch; }

  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
</style>
