<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's *Presets* (UC-WRK-30 checks 3, 4): its saved presets - renamed and withdrawn
  // here, changed from a collection's *Save as preset* -, the preset a new hub starts from, and the
  // settings locked for every hub and collection.
  //
  // **What was made from a preset keeps its settings.** A preset is copied, never followed, so
  // renaming or withdrawing one changes no hub or collection; the screen says so once.
  //
  // **The default is not withdrawn from under the workspace.** Its *Withdraw* stays, unavailable,
  // with the reason (design-system.md §4, case 2) - the server refuses it too.
  //
  // **A lock is a whole set.** `PATCH /tenant/structure` takes every workspace lock at once, so the
  // capability locks - one field of one level, set on the *Structure* panel's locks - travel back
  // as they are and are shown, not offered: this screen locks whole settings. The installation's
  // and the plan's locks are not the workspace's to lift; they are shown with where they come from.

  import { untrack } from 'svelte';

  import { Banner, Button, CheckboxGroup, Input, PageHeader, Select, Spinner, Stack } from '@hubtask/design-system/components';
  import type { Preset, TransportError } from '@hubtask/sync-engine';

  import { presetChoice, presetName, presets, presetValue } from '../lib/data/presets.svelte.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { SETTINGS } from '../lib/workspace/structureDraft.ts';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';

  $effect(() => untrack(() => workspace.open()));
  $effect(() => untrack(() => presets.open()));
  $effect(() => page.entitle(t('app.presets.title')));

  const reading = $derived(workspace.state);
  const structure = $derived(workspace.workspace?.structure);
  const refusal = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let busy = $state<string | undefined>(undefined);

  /** The default as it is chosen: Hubtask's key or a workspace preset's id, or empty for none. */
  const defaultValue = $derived(structure?.default_preset ?? structure?.default_preset_id ?? '');
  const defaultOptions = $derived([
    { value: '', label: t('app.presets.default_none') },
    ...presets.all.filter((preset) => preset.origin !== 'OWN').map((preset) => ({ value: presetValue(preset), label: presetName(preset) })),
  ]);

  /** Names typed for a rename, by preset id. */
  let names = $state<Record<string, string>>({});

  async function act(key: string, work: () => Promise<unknown>, done: string) {
    if (busy) return;
    busy = key;
    failure = undefined;
    try {
      await work();
      toast.show({ text: t(done), tone: 'success' });
    } catch (error) {
      failure = renderProblem(error as TransportError, messages);
    } finally {
      busy = undefined;
    }
  }

  function rename(preset: Preset) {
    const name = (names[preset.id ?? ''] ?? '').trim();
    if (!preset.id || name === '' || name === preset.name) return;
    void act(`rename-${preset.id}`, () => presets.change(preset.id ?? '', { name }, preset.version ?? 0), 'app.presets.renamed');
  }

  function withdraw(preset: Preset) {
    if (!preset.id) return;
    void act(`withdraw-${preset.id}`, () => presets.withdraw(preset.id ?? '', preset.version ?? 0), 'app.presets.withdrawn');
  }

  function chooseDefault(value: string) {
    if (value === defaultValue) return;
    const choice = presetChoice(value);
    const body = choice.preset_id ? { default_preset_id: choice.preset_id } : { default_preset: choice.preset ?? null };
    void act('default', () => workspace.changeStructure(body), 'app.presets.default_saved');
  }

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
      const above = lockedAbove.find((lock) => lock.key === setting);
      return {
        value: setting,
        label: t(`app.structure.setting.${setting}`),
        ...(above ? { disabledReason: t('app.presets.locked_above', { origin: t('app.structure.locked_origin', { origin: above.origin }) }) } : {}),
      };
    }),
  );

  function chooseLocks(selected: readonly string[]) {
    void act('locks', () => workspace.changeStructure({ locks: [...selected, ...fieldLocks] }), 'app.presets.locks_saved');
  }
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.presets.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'presets', label: t('app.presets.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="quiet">
      <Spinner label={t('app.presets.reading')} />
      <span>{t('app.presets.reading')}</span>
    </p>
  {:else if refusal}
    <Banner tone="danger" title={refusal.message}>
      {#if refusal.reference}{t('app.error_reference', { request_id: refusal.reference })}{/if}
    </Banner>
  {:else if structure}
    {#if failure}
      <Banner tone="danger" title={failure.message}>
        {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
      </Banner>
    {/if}

    <WorkspaceGate permission="STRUCTURE">
      <Stack gap="400">
        <section class="block" aria-labelledby="presets-saved" data-presets-saved>
          <h2 id="presets-saved">{t('app.presets.saved_title')}</h2>
          <p class="quiet">{t('app.presets.saved_about')}</p>
          {#if presets.workspace.length === 0}
            <p class="quiet" data-presets-empty>{t('app.presets.none')}</p>
          {:else}
            <ul class="rows">
              {#each presets.workspace as preset (preset.id)}
                {@const isDefault = preset.id === structure.default_preset_id}
                <li class="row" data-preset={preset.id}>
                  <Input
                    label={t('app.presets.name_of', { name: preset.name ?? '' })}
                    maxlength={100}
                    hint={isDefault ? t('app.presets.is_default') : undefined}
                    value={names[preset.id ?? ''] ?? preset.name ?? ''}
                    oninput={(event: Event) => (names = { ...names, [preset.id ?? '']: (event.currentTarget as HTMLInputElement).value })}
                  />
                  <div class="actions">
                    <Button
                      size="sm"
                      tone="secondary"
                      isBusy={busy === `rename-${preset.id}`}
                      busyLabel={t('app.presets.saving')}
                      onclick={() => rename(preset)}
                    >
                      {t('app.presets.rename')}
                    </Button>
                    <Button
                      size="sm"
                      tone="subtle"
                      isBusy={busy === `withdraw-${preset.id}`}
                      busyLabel={t('app.presets.withdrawing')}
                      disabledReason={isDefault ? t('app.presets.withdraw_default') : undefined}
                      aria-label={t('app.presets.withdraw_named', { name: preset.name ?? '' })}
                      onclick={() => withdraw(preset)}
                    >
                      {t('app.presets.withdraw')}
                    </Button>
                  </div>
                </li>
              {/each}
            </ul>
          {/if}
        </section>

        <section class="block" aria-labelledby="presets-default" data-presets-default>
          <h2 id="presets-default">{t('app.presets.default_title')}</h2>
          <Select
            label={t('app.presets.default_label')}
            hint={t('app.presets.default_hint')}
            options={defaultOptions}
            bind:value={() => defaultValue, (value) => chooseDefault(value)}
          />
        </section>

        <section class="block" aria-labelledby="presets-locks" data-presets-locks>
          <h2 id="presets-locks">{t('app.presets.locks_title')}</h2>
          <CheckboxGroup
            label={t('app.presets.locks_label')}
            hint={t('app.presets.locks_hint')}
            options={lockOptions}
            selected={lockedSettings}
            onChange={chooseLocks}
          />
          {#if fieldLocks.length > 0}
            <p class="quiet" data-field-locks>{t('app.presets.field_locks', { count: String(fieldLocks.length) })}</p>
          {/if}
        </section>
      </Stack>
    </WorkspaceGate>
  {/if}
</Stack>

<style>
  .block { display: flex; flex-direction: column; gap: var(--sp-150); }
  .block h2 { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; display: flex; gap: var(--sp-100); align-items: center; }
  .rows { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--sp-200); }
  .row { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-150); }
  .row :global(> :first-child) { flex: 1 1 auto; }
  .actions { display: flex; gap: var(--sp-100); }
</style>
