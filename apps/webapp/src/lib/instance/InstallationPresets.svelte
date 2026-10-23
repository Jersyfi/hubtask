<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The installation's presets on *Installation → Defaults* (UC-INS-18 checks 1, 3): offered to
  // every workspace, one of them the default for new workspaces, the default's locks. Drawn by the
  // screen in multi mode only - a single installation's one workspace keeps its own presets, and a
  // second level of them would be a level nobody else sees (check 4, P-10).
  //
  // **A file that enforces the level enforces the presets.** The list shows, without a control: a
  // save the server refuses would be a control that lies.
  //
  // **The default is not withdrawn from under the workspaces.** Its *Withdraw* stays, unavailable,
  // with the reason (design-system.md §4, case 2) - the server refuses it too.
  //
  // **A lock is a whole set**, and only the default carries one. The screen locks whole settings;
  // a capability lock (one field of one level, from `hubctl` or the instance file) travels back
  // as it is and is counted, not offered.

  import { untrack } from 'svelte';

  import { Banner, Button, CheckboxGroup, Input, Select, Spinner, Stack } from '@hubtask/design-system/components';
  import type { InstallationPreset } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import { BUILT_IN, type BuiltInKey } from '../data/presetChoice.ts';
  import { installationPresets } from '../data/installationPresets.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { SETTINGS } from '../workspace/structureDraft.ts';

  interface Props {
    /** A file enforces the installation's level: the presets are shown, not offered for change. */
    isEnforced: boolean;
  }

  const { isEnforced }: Props = $props();

  $effect(() => untrack(() => installationPresets.open()));

  const reading = $derived(installationPresets.state);
  const unreadable = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  const all = $derived(installationPresets.all);
  const chosenDefault = $derived(installationPresets.default);

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let busy = $state<string | undefined>(undefined);
  /** Names typed for a rename, by preset id. */
  let names = $state<Record<string, string>>({});
  let newName = $state('');
  let newFrom = $state<BuiltInKey>('TASKS');
  let isNameMissing = $state(false);

  const builtInOptions = $derived(BUILT_IN.map((key) => ({ value: key, label: t(`app.structure.preset.${key}`) })));
  const defaultOptions = $derived([
    { value: '', label: t('app.instance.presets_default_none') },
    ...all.map((preset) => ({ value: preset.id, label: preset.name })),
  ]);

  /** The default's locks on a whole setting, and on one capability of one level. */
  const lockedSettings = $derived((chosenDefault?.locks ?? []).filter((key) => !key.includes('.')));
  const capabilityLocks = $derived((chosenDefault?.locks ?? []).filter((key) => key.includes('.')));
  const lockOptions = $derived(SETTINGS.map((setting) => ({ value: setting, label: t(`app.structure.setting.${setting}`) })));

  /** The default's locks in words, for the read-only list. */
  function locksSaid(preset: InstallationPreset): string {
    const whole = preset.locks.filter((key) => !key.includes('.'));
    const words = whole.map((key) => t(`app.structure.setting.${key}`));
    const fields = preset.locks.length - whole.length;
    if (fields > 0) words.push(t('app.instance.presets_field_locks_short', { count: String(fields) }));
    return words.join(', ');
  }

  async function act(key: string, work: () => Promise<unknown>, done: string): Promise<boolean> {
    if (busy) return false;
    busy = key;
    failure = undefined;
    try {
      await work();
      toast.show({ text: t(done), tone: 'success' });
      return true;
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
      return false;
    } finally {
      busy = undefined;
    }
  }

  async function create(): Promise<void> {
    const name = newName.trim();
    isNameMissing = name === '';
    if (isNameMissing) return;
    if (await act('create', () => installationPresets.create({ name, preset: newFrom }), 'app.instance.presets_created')) newName = '';
  }

  function rename(preset: InstallationPreset): void {
    const name = (names[preset.id] ?? '').trim();
    if (name === '' || name === preset.name) return;
    void act(`rename-${preset.id}`, () => installationPresets.change(preset.id, { name }, preset.version), 'app.instance.presets_renamed');
  }

  function startAgain(preset: InstallationPreset, key: string): void {
    void act(`from-${preset.id}`, () => installationPresets.change(preset.id, { preset: key as BuiltInKey }, preset.version), 'app.instance.presets_settings_saved');
  }

  function chooseDefault(value: string): void {
    if (value === (chosenDefault?.id ?? '')) return;
    const target = value === '' ? chosenDefault : all.find((preset) => preset.id === value);
    if (!target) return;
    void act('default', () => installationPresets.change(target.id, { is_default: value !== '' }, target.version), 'app.instance.presets_default_saved');
  }

  function chooseLocks(selected: readonly string[]): void {
    const target = chosenDefault;
    if (!target) return;
    void act('locks', () => installationPresets.change(target.id, { locks: [...selected, ...capabilityLocks] }, target.version), 'app.instance.presets_locks_saved');
  }

  function withdraw(preset: InstallationPreset): void {
    void act(`withdraw-${preset.id}`, () => installationPresets.withdraw(preset.id, preset.version), 'app.instance.presets_withdrawn');
  }
</script>

<section class="block" aria-labelledby="installation-presets" data-installation-presets>
  <h2 id="installation-presets" class="section">{t('app.instance.presets_title')}</h2>
  <p class="quiet">{t('app.instance.presets_about')}</p>

  {#if failure}
    <Banner tone="danger" title={failure.message}>
      {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
    </Banner>
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
  {:else if isEnforced}
    {#if all.length === 0}
      <p class="quiet" data-presets-empty>{t('app.instance.presets_none')}</p>
    {:else}
      <ul class="rows">
        {#each all as preset (preset.id)}
          <li class="read" data-installation-preset={preset.id}>
            <span>{preset.name}</span>
            {#if preset.is_default}<span class="mark">{t('app.instance.presets_is_default')}</span>{/if}
            {#if preset.locks.length > 0}<span class="mark">{t('app.instance.presets_locks_said', { locks: locksSaid(preset) })}</span>{/if}
          </li>
        {/each}
      </ul>
    {/if}
  {:else}
    <Stack gap="300">
      {#if all.length === 0}
        <p class="quiet" data-presets-empty>{t('app.instance.presets_none')}</p>
      {:else}
        <ul class="rows">
          {#each all as preset (preset.id)}
            <li class="row" data-installation-preset={preset.id}>
              <Input
                label={t('app.instance.presets_name_of', { name: preset.name })}
                maxlength={100}
                hint={preset.is_default ? t('app.instance.presets_is_default') : undefined}
                value={names[preset.id] ?? preset.name}
                oninput={(event: Event) => (names = { ...names, [preset.id]: (event.currentTarget as HTMLInputElement).value })}
              />
              <Select
                label={t('app.instance.presets_start_again')}
                placeholder={t('app.instance.presets_start_again_placeholder')}
                options={builtInOptions}
                bind:value={() => '', (value) => value && startAgain(preset, value)}
              />
              <div class="actions">
                <Button
                  size="sm"
                  tone="secondary"
                  isBusy={busy === `rename-${preset.id}`}
                  busyLabel={t('app.instance.working')}
                  onclick={() => rename(preset)}
                >
                  {t('app.presets.rename')}
                </Button>
                <Button
                  size="sm"
                  tone="subtle"
                  isBusy={busy === `withdraw-${preset.id}`}
                  busyLabel={t('app.presets.withdrawing')}
                  disabledReason={preset.is_default ? t('app.instance.presets_withdraw_default') : undefined}
                  aria-label={t('app.presets.withdraw_named', { name: preset.name })}
                  onclick={() => withdraw(preset)}
                >
                  {t('app.presets.withdraw')}
                </Button>
              </div>
            </li>
          {/each}
        </ul>

        <Select
          label={t('app.instance.presets_default_label')}
          hint={t('app.instance.presets_default_hint')}
          options={defaultOptions}
          bind:value={() => chosenDefault?.id ?? '', (value) => chooseDefault(value)}
        />

        {#if chosenDefault}
          <div data-installation-preset-locks>
            <CheckboxGroup
              label={t('app.instance.presets_locks_label', { name: chosenDefault.name })}
              hint={t('app.instance.presets_locks_hint')}
              options={lockOptions}
              selected={lockedSettings}
              onChange={chooseLocks}
            />
            {#if capabilityLocks.length > 0}
              <p class="quiet">{t('app.instance.presets_field_locks', { count: String(capabilityLocks.length) })}</p>
            {/if}
          </div>
        {/if}
      {/if}

      <form
        class="create"
        data-installation-preset-create
        onsubmit={(event) => {
          event.preventDefault();
          void create();
        }}
      >
        <Input
          label={t('app.instance.presets_new_name')}
          maxlength={100}
          value={newName}
          error={isNameMissing ? t('app.forms.required') : undefined}
          oninput={(event: Event) => {
            newName = (event.currentTarget as HTMLInputElement).value;
            isNameMissing = false;
          }}
        />
        <Select label={t('app.instance.presets_new_from')} options={builtInOptions} bind:value={() => newFrom, (value) => (newFrom = value as BuiltInKey)} />
        <div class="actions">
          <Button type="submit" tone="primary" isBusy={busy === 'create'} busyLabel={t('app.instance.working')}>
            {t('app.instance.presets_create')}
          </Button>
        </div>
      </form>
    </Stack>
  {/if}
</section>

<style>
  .block { display: flex; flex-direction: column; gap: var(--sp-150); max-inline-size: 80ch; }
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  .quiet { margin: 0; color: var(--text-secondary); max-inline-size: 64ch; display: flex; gap: var(--sp-100); align-items: center; }
  .rows { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--sp-200); }
  .row, .create { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-150); margin: 0; }
  .row :global(> :first-child), .create :global(> :first-child) { flex: 1 1 auto; }
  .read { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  .mark { color: var(--text-secondary); font-size: var(--fs-100); }
  .actions { display: flex; gap: var(--sp-100); }
</style>
