<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Templates* at the installation (user-experience.md §8.4, UC-INS-18): the presets every
  // workspace is offered, as a list page (page-system §7.1, §9.4). *New preset* the head's primary
  // (Dialog M), the default for new workspaces and its locks a Dialog M beside it; a preset's
  // rename, its fresh start from a built-in and its withdrawal in its row's ⋯, each a Dialog S. In
  // enforce mode the file is the source: the presets are listed and nothing is offered for change.

  import { untrack } from 'svelte';

  import { Button, CheckboxGroup, DataTable, Dialog, EmptyState, FilterBar, Input, ListPage, Select, Spinner, Stack, type DataColumn, type MenuItem } from '@hubtask/design-system/components';
  import type { InstallationPreset } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import InstanceGate from '../lib/instance/InstanceGate.svelte';
  import { BUILT_IN, type BuiltInKey } from '../lib/data/presetChoice.ts';
  import { installationPresets } from '../lib/data/installationPresets.svelte.ts';
  import { instance } from '../lib/data/instance.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { installationTrail } from '../lib/instance/trail.ts';
  import { matchesSearch } from '../lib/page/list.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { SETTINGS } from '../lib/workspace/structureDraft.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  const { onnavigate }: { onnavigate: (path: string) => void } = $props();

  // An installation configured from a file in enforce mode offers no write here either.
  $effect(() => {
    if (!instance.isElevated) return;
    return untrack(() => instance.openSettings());
  });
  const isEnforced = $derived(instance.settings.status === 'ready' && instance.settings.data.is_enforced_from_file === true);
  /** A level a single installation does not have (UC-INS-18 check 4). */
  const hasLevel = $derived(manifest.isMultiTenant);

  $effect(() => page.entitle(t('app.instance.templates')));

  // A single installation has no level of its own, so nothing is read (UC-INS-18 check 4).
  $effect(() => {
    if (!hasLevel) return;
    return untrack(() => installationPresets.open());
  });

  const reading = $derived(installationPresets.state);
  const unreadable = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  const all = $derived(installationPresets.all);
  const chosenDefault = $derived(installationPresets.default);

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let busy = $state<string | undefined>(undefined);
  let isCreating = $state(false);
  let isDefaultsOpen = $state(false);
  let renaming = $state<InstallationPreset | undefined>(undefined);
  let renamed = $state('');
  let restarting = $state<InstallationPreset | undefined>(undefined);
  let restartFrom = $state<BuiltInKey>('TASKS');
  let withdrawing = $state<InstallationPreset | undefined>(undefined);
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
          : { message: messages.t('app.something_went_wrong'), fields: new Map(), isServerFault: true };
      return false;
    } finally {
      busy = undefined;
    }
  }

  async function create(): Promise<void> {
    const name = newName.trim();
    isNameMissing = name === '';
    if (isNameMissing) return;
    if (await act('create', () => installationPresets.create({ name, preset: newFrom }), 'app.instance.presets_created')) {
      newName = '';
      isCreating = false;
    }
  }

  async function rename(): Promise<void> {
    const preset = renaming;
    if (!preset) return;
    const name = renamed.trim();
    if (name === '' || name === preset.name) {
      renaming = undefined;
      return;
    }
    if (await act(`rename-${preset.id}`, () => installationPresets.change(preset.id, { name }, preset.version), 'app.instance.presets_renamed')) renaming = undefined;
  }

  async function startAgain(): Promise<void> {
    const preset = restarting;
    if (!preset) return;
    if (await act(`from-${preset.id}`, () => installationPresets.change(preset.id, { preset: restartFrom }, preset.version), 'app.instance.presets_settings_saved')) restarting = undefined;
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

  async function withdraw(): Promise<void> {
    const preset = withdrawing;
    if (!preset) return;
    if (await act(`withdraw-${preset.id}`, () => installationPresets.withdraw(preset.id, preset.version), 'app.instance.presets_withdrawn')) withdrawing = undefined;
  }

  let search = $state('');
  const shown = $derived(all.filter((preset) => matchesSearch(search, [preset.name])));

  const columns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.instance.presets_column_name'), priority: 1 },
    { id: 'locks', label: t('app.instance.presets_column_locks'), priority: 2 },
  ]);

  function rowMenu(preset: InstallationPreset) {
    if (isEnforced) return undefined;
    const items: MenuItem[] = [
      { id: 'rename', label: t('app.presets.rename') },
      { id: 'start-again', label: t('app.instance.presets_start_again_placeholder') },
      { id: 'withdraw', label: t('app.presets.withdraw'), isDestructive: true, hasSeparatorBefore: true, disabledReason: preset.is_default ? t('app.instance.presets_withdraw_default') : undefined },
    ];
    return { label: t('app.page.more_for', { name: preset.name }), items };
  }

  function onRowMenu(preset: InstallationPreset, id: string): void {
    failure = undefined;
    if (id === 'rename') {
      renamed = preset.name;
      renaming = preset;
    } else if (id === 'start-again') {
      restartFrom = 'TASKS';
      restarting = preset;
    } else if (id === 'withdraw') {
      withdrawing = preset;
    }
  }

  let createForm = $state<HTMLFormElement | undefined>(undefined);
  let renameForm = $state<HTMLFormElement | undefined>(undefined);
</script>

<InstanceGate onleave={() => onnavigate('/')}>
  {#snippet children(door)}
    <ListPage
      head={{
        title: t('app.instance.templates'),
        breadcrumb: installationTrail({ id: 'screen', label: t('app.instance.templates') }),
        primary: hasLevel && !isEnforced
          ? { label: t('app.instance.presets_new'), icon: 'plus', onclick: () => { newName = ''; isNameMissing = false; failure = undefined; isCreating = true; } }
          : undefined,
        secondary: hasLevel && !isEnforced && all.length > 0 ? [{ label: t('app.instance.presets_defaults'), onclick: () => { failure = undefined; isDefaultsOpen = true; } }] : [],
      }}
      refused={door}
    >
      {#snippet intro()}<p class="quiet">{t(hasLevel ? 'app.instance.presets_about' : 'app.instance.presets_single')}</p>{/snippet}
      {#snippet filters()}
        {#if hasLevel}
          <FilterBar
            searchLabel={t('app.instance.presets_search')}
            clearSearchLabel={t('app.page.clear_search')}
            bind:search
            countLabel={reading.status === 'ready' ? t('app.page.found', { shown: String(shown.length), count: String(all.length) }) : undefined}
          />
        {/if}
      {/snippet}

      <!-- A level a single installation does not have (UC-INS-18 check 4): no rows. -->
      {#if hasLevel}
        <div class="presets" data-installation-presets>
          {#if failure}<FailureNote problem={failure} />{/if}
          {#if reading.status === 'loading' || reading.status === 'idle'}
            <p class="quiet"><Spinner label={t('app.instance.reading')} /><span>{t('app.instance.reading')}</span></p>
          {:else if unreadable}
            <FailureNote problem={unreadable} />
          {:else}
            <DataTable
              label={t('app.instance.presets_title')}
              {columns}
              rows={shown}
              key={(preset: InstallationPreset) => preset.id}
              rowLabel={(preset: InstallationPreset) => preset.name}
              menu={rowMenu}
              onmenu={onRowMenu}
              actionsLabel={t('app.page.row_actions')}
              sheetDismissLabel={t('app.page.sheet_close')}
              sheetGrabLabel={t('app.page.sheet_grab')}
              shownLabel={all.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
            >
              {#snippet cell(preset: InstallationPreset, column: DataColumn)}
                {#if column.id === 'name'}
                  <span class="name" data-installation-preset={preset.id}>{preset.name}</span>
                  {#if preset.is_default}<span class="note">{t('app.instance.presets_is_default')}</span>{/if}
                {:else}
                  <span class="quiet">{preset.locks.length > 0 ? locksSaid(preset) : ''}</span>
                {/if}
              {/snippet}
              {#snippet empty()}
                {#if all.length === 0}
                  <EmptyState kind="unused" title={t('app.instance.presets_none')} />
                {:else}
                  <EmptyState kind="filtered" title={t('app.instance.presets_none_found')}>
                    {#snippet action()}<Button onclick={() => (search = '')}>{t('app.page.clear')}</Button>{/snippet}
                  </EmptyState>
                {/if}
              {/snippet}
            </DataTable>
          {/if}
        </div>
      {/if}
    </ListPage>
  {/snippet}
</InstanceGate>

<Dialog
  size="m"
  title={t('app.instance.presets_new')}
  isOpen={isCreating}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (isCreating = false)}
  primary={{ label: t('app.instance.presets_create'), isBusy: busy === 'create', busyLabel: t('app.instance.working'), onclick: () => createForm?.requestSubmit() }}
>
  <form bind:this={createForm} data-installation-preset-create onsubmit={(event) => { event.preventDefault(); void create(); }}>
    <Stack gap="150">
      {#if failure}<FailureNote problem={failure} />{/if}
      <Input
        label={t('app.instance.presets_new_name')}
        maxlength={100}
        bind:value={newName}
        error={isNameMissing && !newName.trim() ? t('app.forms.required') : undefined}
        isRequired
      />
      <Select label={t('app.instance.presets_new_from')} options={builtInOptions} bind:value={() => newFrom, (value) => (newFrom = value as BuiltInKey)} />
    </Stack>
  </form>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isCreating = false)}>{t('app.instance.cancel')}</Button>
  {/snippet}
</Dialog>

<Dialog
  size="m"
  title={t('app.instance.presets_defaults')}
  isOpen={isDefaultsOpen}
  dismissLabel={t('app.page.close')}
  onClose={() => (isDefaultsOpen = false)}
>
  <Stack gap="150">
    {#if failure}<FailureNote problem={failure} />{/if}
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
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isDefaultsOpen = false)}>{t('app.page.close')}</Button>
  {/snippet}
</Dialog>

<Dialog
  size="s"
  title={t('app.instance.presets_rename_title', { name: renaming?.name ?? '' })}
  isOpen={renaming !== undefined}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (renaming = undefined)}
  primary={{ label: t('app.presets.rename'), isBusy: busy === `rename-${renaming?.id}`, busyLabel: t('app.instance.working'), onclick: () => renameForm?.requestSubmit() }}
>
  <form bind:this={renameForm} onsubmit={(event) => { event.preventDefault(); void rename(); }}>
    <Stack gap="150">
      {#if failure}<FailureNote problem={failure} />{/if}
      <Input label={t('app.instance.presets_name_of', { name: renaming?.name ?? '' })} maxlength={100} bind:value={renamed} isRequired />
    </Stack>
  </form>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (renaming = undefined)}>{t('app.instance.cancel')}</Button>
  {/snippet}
</Dialog>

<Dialog
  size="s"
  title={t('app.instance.presets_start_again_title', { name: restarting?.name ?? '' })}
  isOpen={restarting !== undefined}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (restarting = undefined)}
  primary={{ label: t('app.instance.presets_start_again_confirm'), isBusy: busy === `from-${restarting?.id}`, busyLabel: t('app.instance.working'), onclick: () => void startAgain() }}
>
  <Stack gap="150">
    {#if failure}<FailureNote problem={failure} />{/if}
    <Select label={t('app.instance.presets_start_again')} options={builtInOptions} bind:value={() => restartFrom, (value) => (restartFrom = value as BuiltInKey)} />
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (restarting = undefined)}>{t('app.instance.cancel')}</Button>
  {/snippet}
</Dialog>

<Dialog
  size="s"
  title={t('app.presets.withdraw_named', { name: withdrawing?.name ?? '' })}
  isOpen={withdrawing !== undefined}
  dismissLabel={t('app.instance.cancel')}
  onClose={() => (withdrawing = undefined)}
  primary={{ label: t('app.presets.withdraw'), tone: 'danger', isBusy: busy === `withdraw-${withdrawing?.id}`, busyLabel: t('app.presets.withdrawing'), onclick: () => void withdraw() }}
>
  <Stack gap="150">
    {#if failure}<FailureNote problem={failure} />{/if}
    <p class="quiet">{t('app.instance.presets_withdraw_cost')}</p>
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (withdrawing = undefined)}>{t('app.instance.cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .presets { display: grid; gap: var(--sp-150); min-inline-size: 0; }
  .name { display: block; font-weight: var(--fw-medium); }
  .note { display: block; color: var(--text-secondary); font-size: var(--fs-100); }
  form { margin: 0; }
</style>
