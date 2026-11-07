<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's *Templates* (configuration/ui.md §4): the gallery of every template by origin,
  // above the presets it holds as their structure part (UC-WRK-30 check 3): its saved presets -
  // renamed and withdrawn here, changed from a collection's *Save as preset*. The preset a new hub
  // starts from and the locks are the workspace screen's *New hubs* (page-system §9.1).
  //
  // **What was made from a preset keeps its settings.** A preset is copied, never followed, so
  // renaming or withdrawing one changes no hub or collection; the screen says so once.
  //
  // **The default is not withdrawn from under the workspace.** Its *Withdraw* stays, unavailable,
  // with the reason (design-system.md §4, case 2) - the server refuses it too.

  import { untrack } from 'svelte';

  import { EmptyState, Button, Input, PageHeader, Spinner, Stack } from '@hubtask/design-system/components';
  import type { Preset, TransportError } from '@hubtask/sync-engine';

  import { presets } from '../lib/data/presets.svelte.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import TemplatesGallery from '../lib/settings/TemplatesGallery.svelte';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';
  import ViewSectionsAsFile from '../lib/settings/ViewSectionsAsFile.svelte';
  import { pagePart } from '../lib/settings/asFile.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  const { onnavigate, template }: { onnavigate?: (path: string) => void; template?: string } = $props();

  $effect(() => untrack(() => workspace.open()));
  $effect(() => untrack(() => presets.open()));
  $effect(() => page.entitle(t('app.presets.title')));

  const reading = $derived(workspace.state);
  const structure = $derived(workspace.workspace?.structure);
  const refusal = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let busy = $state<string | undefined>(undefined);

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
  <div class="file"><ViewSectionsAsFile {...pagePart('/administration/presets')} /></div>

  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="quiet">
      <Spinner label={t('app.presets.reading')} />
      <span>{t('app.presets.reading')}</span>
    </p>
  {:else if refusal}
    <FailureNote problem={refusal} />
  {:else if structure}
    {#if failure}
      <FailureNote problem={failure} />
    {/if}

    <WorkspaceGate permission="STRUCTURE">
      <Stack gap="400">
        <TemplatesGallery {onnavigate} preselect={template} />

        <section class="block" aria-labelledby="presets-saved" data-presets-saved>
          <h2 id="presets-saved">{t('app.presets.saved_title')}</h2>
          <p class="quiet">{t('app.presets.saved_about')}</p>
          {#if presets.workspace.length === 0}
            <div data-presets-empty><EmptyState kind="unused" title={t('app.presets.none')} /></div>
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
  .file { display: flex; justify-content: flex-end; }
</style>
