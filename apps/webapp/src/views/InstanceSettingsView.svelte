<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installation's own values, and where each lock came from (SI-17, ADR-0070 §2, §5).
  //
  // **Read and written here**, because the API is the product and this is one of its three doors:
  // "hubctl und das Dashboard sind zwei Clients davon — dieselbe Regel, die für die Anwendung gilt".
  // An operator who can see a switch and has to reach for a terminal to move it is an operator for
  // whom the dashboard is a status page.
  //
  // **Except where the configuration says otherwise**, which is the other half of the same section:
  // a file in `enforce` mode is the source, the writing routes refuse, and this screen then offers
  // no controls at all rather than offering a refusal. `is_enforced_from_file` is what says so, and
  // it is the server's answer rather than this screen's guess.
  //
  // **Every switch carries its lock.** Open means a workspace may tighten it; locked means the value
  // applies and the workspace's control is off — "ein geschlossenes Feld ist sichtbar mit Wert,
  // Schloss und 'Set by the installation' — nie unsichtbar" (§5.3). A workspace sees the same fact
  // from the other side on its own sign-in screen.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, Input, PageHeader, Spinner, Stack, Switch, Table } from '@hubtask/design-system/components';
  import type { InstanceSetting, InstanceSettings } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { instance } from '../lib/data/instance.svelte.ts';
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

  const columns = $derived([
    { id: 'switch', label: t('app.instance.column_switch') },
    { id: 'value', label: t('app.instance.column_value') },
    { id: 'lock', label: t('app.instance.column_lock') },
  ]);

  /** The two areas as one list of rows, each carrying which area it came from. */
  const rows = $derived([
    ...Object.entries(settings?.sign_in ?? {}).map(([key, setting]) => ({ area: 'sign_in', key, setting })),
    ...Object.entries(settings?.legal ?? {}).map(([key, setting]) => ({ area: 'legal', key, setting })),
  ]);

  /** A value as one line. The switches are numbers, flags, strings and lists of strings. */
  function shown(value: unknown): string {
    if (Array.isArray(value)) return value.join(', ');
    if (value === null || value === undefined) return '';
    return String(value);
  }

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

  /** Reads one switch out of the draft, and writes it back whole. */
  function setting(area: 'sign_in' | 'legal', key: string): InstanceSetting | undefined {
    return draft?.[area]?.[key];
  }

  function put(area: 'sign_in' | 'legal', key: string, next: InstanceSetting): void {
    if (!draft) return;
    draft = { ...draft, [area]: { ...(draft[area] ?? {}), [key]: next } };
  }

  /**
   * Removes a switch from the draft.
   *
   * `PUT` replaces the level whole, so a switch the operator cleared is one the installation stops
   * deciding — and a workspace that had been told the value goes back to deciding for itself. That
   * is what "the write replaces" means and why clearing has a control of its own.
   */
  function clear(area: 'sign_in' | 'legal', key: string): void {
    if (!draft) return;
    const kept = { ...(draft[area] ?? {}) };
    delete kept[key];
    draft = { ...draft, [area]: kept };
  }

  async function save(): Promise<void> {
    if (!draft) return;
    isWorking = true;
    failure = undefined;
    saved = false;
    try {
      await instance.writeSettings({
        sign_in: draft.sign_in,
        legal: draft.legal,
        blocklist_file: draft.blocklist_file,
      });
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
        {#if settings?.blocklist_file}
          <p class="quiet-line">{t('app.instance.blocklist_file', { path: settings.blocklist_file })}</p>
        {/if}

        {#if draft}
          <form
            class="panel"
            onsubmit={(event) => {
              event.preventDefault();
              void save();
            }}
          >
            <Stack gap="200">
              <h2 class="section">{t('app.instance.settings_edit_title')}</h2>
              <p class="quiet-line">{t('app.instance.settings_edit_hint')}</p>

              {#each rows as row (row.area + '.' + row.key)}
                {@const current = setting(row.area as 'sign_in' | 'legal', row.key)}
                <div class="field">
                  <Input
                    label={row.area + '.' + row.key}
                    value={current === undefined ? '' : shown(current.value)}
                    oninput={(event) => {
                      const raw = (event.currentTarget as HTMLInputElement).value;
                      if (raw.trim() === '') {
                        clear(row.area as 'sign_in' | 'legal', row.key);
                        return;
                      }
                      // The value keeps the kind the server answered: a number stays a number, a
                      // flag a flag. A switch retyped into a string would be a switch the domain
                      // refuses on the way back in.
                      const was = row.setting.value;
                      const value =
                        typeof was === 'number'
                          ? Number(raw)
                          : typeof was === 'boolean'
                            ? raw === 'true'
                            : raw;
                      put(row.area as 'sign_in' | 'legal', row.key, {
                        value,
                        locked: current?.locked ?? false,
                      });
                    }}
                    autocomplete="off"
                    spellcheck={false}
                  />
                  <Switch
                    label={t('app.instance.locked_label')}
                    hint={t('app.instance.locked_hint')}
                    checked={current?.locked === true}
                    onchange={(event) =>
                      put(row.area as 'sign_in' | 'legal', row.key, {
                        value: current?.value ?? row.setting.value,
                        locked: (event.currentTarget as HTMLInputElement).checked,
                      })}
                  />
                </div>
              {/each}

              <Input
                label={t('app.instance.blocklist_label')}
                hint={t('app.instance.blocklist_hint')}
                value={draft.blocklist_file ?? ''}
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
          <Table label={t('app.instance.settings')} isLabelHidden columns={columns}>
            {#each rows as row (row.area + '.' + row.key)}
              <tr>
                <td class="mono">{row.area}.{row.key}</td>
                <td>{shown(row.setting.value)}</td>
                <td>
                  <Badge tone={row.setting.locked ? 'warning' : 'neutral'}>
                    {row.setting.locked ? t('app.instance.lock_locked') : t('app.instance.lock_open')}
                  </Badge>
                </td>
              </tr>
            {/each}
          </Table>
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
  .mono { font-family: var(--font-mono); font-size: var(--fs-100); overflow-wrap: anywhere; }
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  /* A form is neither prose nor a table (ADR-0065 decision 2): the fields carry a measure of their
     own while the table beside them takes the width. */
  form { margin: 0; max-inline-size: 60ch; }
  .field { display: grid; gap: var(--sp-050); padding-block-end: var(--sp-100); border-block-end: var(--bw-hairline) solid var(--border-subtle); }

  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
</style>
