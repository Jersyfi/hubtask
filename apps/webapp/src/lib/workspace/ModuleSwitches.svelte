<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The module switches of one place (UC-INS-21): the workspace under *How we work → Modules*, or
  // one hub in its settings. One switch per module, with where its state comes from and whether a
  // level above locked it; a hub only switches off what the workspace has on (check 3).
  //
  // **A switch acts at once**, as every single switch of the administration does - one row is not
  // a form. Switching off asks first and says that the data is kept and hidden (check 4); switching
  // on needs no configuration step (check 2), so it does not ask.
  //
  // **What a level above locked is not offered** (check 6): the control is switched off with the
  // reason, and the chip says whose value it is and whom to ask. A value set here offers *Reset*,
  // which hands the module back to the level above.

  import { untrack } from 'svelte';

  import { Button, Dialog, IconButton, Inline, SettingRow, Spinner, Switch } from '@hubtask/design-system/components';
  import type { ConfigTemplate, ConfigTemplateList, ModuleList, ModuleReach, ModuleState, ResourceState } from '@hubtask/sync-engine';

  import ValueSource from './ValueSource.svelte';
  import ViewAsFile from '../settings/ViewAsFile.svelte';
  import SectionFindings from '../settings/SectionFindings.svelte';
  import { asksFirst, isSetHere, moduleCodes, modules, offReach, unavailableBecause } from '../data/modules.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { newMarks } from '../product/newmark.svelte.ts';
  import { settingsOf, startsOf, switchedOn } from '../product/switchedon.ts';
  import { holdsHere, hubCreationHere } from '../data/capability.svelte.ts';
  import { config } from '../data/config.svelte.ts';
  import CreateContainerDialog from './CreateContainerDialog.svelte';

  interface Props {
    /** The hub to switch for; absent is the workspace. */
    hubId?: string;
    /** Where a hub made from a *Start with…* template is opened. */
    onnavigate?: (path: string) => void;
    /** The workspace's *View as file*, opened from the page's head. */
    isFileOpen?: boolean;
    /** Whether the head draws *View as file*, so the list draws no trigger of its own. */
    isTriggerHidden?: boolean;
  }

  let { hubId, onnavigate, isFileOpen = $bindable(false), isTriggerHidden = false }: Props = $props();
  const place = $derived(hubId ? 'HUB' : 'WORKSPACE');

  let reading = $state<ResourceState<ModuleList>>({ status: 'idle' });
  $effect(() => {
    const at = hubId;
    return untrack(() => modules.at(at, (next) => (reading = next)));
  });
  const all = $derived(reading.status === 'ready' ? reading.data.modules : []);
  /** The page's values as its part of the configuration document (configuration/ui.md A10). */
  const shown = $derived(
    all.map((state) => ({
      key: `module.${state.module.toLowerCase()}`,
      value: state.on,
      source: state.source,
      lockedBy: state.locked_by,
    })),
  );
  const readRefusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  /**
   * What each switch draws: the saved value, the reader's choice while it is written, and the
   * saved value again once the write fails or the switch-off waits for its question. Bound, so a
   * revert redraws the switch - a box set back by hand under a bound value stayed drawn off
   * after the write had switched the module on.
   */
  let drawn = $state<Record<string, boolean>>({});
  $effect(() => {
    drawn = Object.fromEntries(all.map((state) => [state.module, state.on]));
  });

  /** The module being switched, so its row shows progress. */
  let busy = $state<ModuleState['module'] | undefined>(undefined);
  /** The module whose switch-off is being asked about. */
  let confirming = $state<ModuleState | undefined>(undefined);
  let isConfirming = $state(false);
  /** Whom and what the switch-off being asked about reaches; the question stands without it. */
  let reach = $state<ResourceState<ModuleReach>>({ status: 'idle' });
  $effect(() => {
    const module = confirming?.module;
    const at = hubId;
    reach = { status: 'idle' };
    if (!module) return;
    return untrack(() => modules.reach(module, at, (next) => (reach = next)));
  });
  const reachLines = $derived(offReach(reach.status === 'ready' ? reach.data : undefined));
  let refusal = $state<{ module: ModuleState['module']; message: string } | undefined>(undefined);
  /** The module just switched on here, whose card says where it is now (user-experience.md §2.8). */
  let justOn = $state<ModuleState['module'] | undefined>(undefined);

  // The card's *Start with…* (§2.8) reads the catalogue, which only a `STRUCTURE` holder may; at
  // the workspace a press makes a hub from the template, so it is offered where a hub may be made.
  let catalogue = $state<readonly ConfigTemplate[]>([]);
  const mayStart = $derived(holdsHere('STRUCTURE') === true && (hubId !== undefined || hubCreationHere() === 'shared'));
  $effect(() => {
    if (!justOn || !mayStart) return;
    return untrack(() => config.templates((next: { status: string; data?: ConfigTemplateList }) => {
      if (next.status === 'ready' && Array.isArray(next.data?.data)) catalogue = next.data.data;
    }));
  });
  const templateName = (template: ConfigTemplate): string =>
    template.origin === 'HUBTASK' ? t(`app.structure.preset.${template.key.toUpperCase()}`) : template.name;
  /** The template a new hub is made from, chosen on the card at the workspace. */
  let startingFrom = $state<string | undefined>(undefined);
  let isCreatingHub = $state(false);
  function created(id: string, landing?: string): void {
    startingFrom = undefined;
    onnavigate?.(landing ?? `/hubs/${id}`);
  }

  async function send(state: ModuleState, to: 'ON' | 'OFF' | 'INHERIT') {
    refusal = undefined;
    busy = state.module;
    try {
      await modules.switch(state.module, to, hubId);
      justOn = to === 'ON' ? state.module : justOn === state.module ? undefined : justOn;
      if (to === 'ON' && switchedOn(state.module, hubId).find) newMarks.add(state.module);
      const name = t(moduleCodes(state).name);
      const code = to === 'ON' ? 'app.modules.switched_on' : to === 'OFF' ? 'app.modules.switched_off' : 'app.modules.reset_done';
      toast.show({ text: t(code, { module: name }), tone: 'success' });
    } catch (error) {
      drawn[state.module] = state.on;
      refusal = { module: state.module, message: renderProblem(error as never, messages).message };
    } finally {
      busy = undefined;
    }
  }

  function change(state: ModuleState, event: Event) {
    // Kept before anything else: the read stays what is in force until the server answers.
    const control = event.currentTarget as HTMLInputElement;
    const next = control.checked;
    if (asksFirst(state, next)) {
      control.checked = state.on;
      drawn[state.module] = state.on;
      confirming = state;
      isConfirming = true;
      return;
    }
    drawn[state.module] = next;
    void send(state, next ? 'ON' : 'OFF');
  }

  function confirmOff() {
    const state = confirming;
    isConfirming = false;
    confirming = undefined;
    if (state) void send(state, 'OFF');
  }
</script>

<div class="switches">
  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="waiting">
      <Spinner label={t('app.modules.reading')} />
      <span>{t('app.modules.reading')}</span>
    </p>
  {:else if readRefusal}
    <FailureNote problem={readRefusal} />
  {:else}
    <!-- One setting row per module (page-system §9.1): at the workspace the page's section is the
         region; at a hub, among the hub's other settings, the rows draw their own. -->
    <div class="modules" class:region={hubId !== undefined}>
      {#each all as state (state.module)}
        {@const codes = moduleCodes(state)}
        {@const reason = unavailableBecause(state, place)}
        <!-- The product's settings beside its switch, at the workspace and while it is on (§8.2). -->
        {@const settings = place === 'WORKSPACE' && state.on ? settingsOf(state.module, (permission) => holdsHere(permission) === true) : undefined}
        <div class="module" data-module={state.module} data-setting={`module.${state.module.toLowerCase()}`}>
          <SettingRow label={t(codes.name)} help={t(codes.hint)} fieldWidth="md">
            {#snippet source()}
              <ValueSource
                source={state.source}
                lock={state.locked_by}
                atHub={place === 'HUB'}
                value={state.on ? t('app.signin_settings.on') : t('app.signin_settings.off')}
              />
              <!-- *Reset* stands beside "Set here", the words it undoes. -->
              {#if isSetHere(state, place)}
                <Button tone="subtle" size="sm" data-reset={state.module} onclick={() => send(state, 'INHERIT')}>
                  {t('app.modules.reset')}
                </Button>
              {/if}
            {/snippet}
            {#snippet control(nameId)}
              <span class="acts">
                <Switch
                  label={t(codes.name)}
                  isLabelHidden
                  aria-labelledby={nameId}
                  bind:checked={() => drawn[state.module] ?? state.on, (value) => (drawn[state.module] = value)}
                  onchange={(event) => change(state, event)}
                  isBusy={busy === state.module}
                  busyLabel={t('app.workspace.saving')}
                  disabledReason={reason ? t(reason) : undefined}
                />
                {#if settings}<a class="act" href={settings} data-module-settings={state.module}>{t('app.modules.settings_of', { module: t(codes.name) })}</a>{/if}
              </span>
            {/snippet}
          </SettingRow>
          {#if refusal?.module === state.module}<p class="refusal" role="alert">{refusal.message}</p>{/if}
          {#if justOn === state.module && state.on}
            {@const next = switchedOn(state.module, hubId)}
            {@const starts = mayStart ? startsOf(state.module, catalogue, hubId) : []}
            <!-- The same note for every module: where it is now, and what it still needs. A tinted
                 band of the row, not a card inside the region (page-system §4.2). -->
            <section class="card" aria-label={t('app.modules.on_card', { module: t(codes.name) })} data-switched-on={state.module}>
              <Inline justify="between" align="center">
                <p class="card-title">{t('app.modules.on_card', { module: t(codes.name) })}</p>
                <IconButton icon="x" size="sm" label={t('app.modules.on_card_close')} onclick={() => (justOn = undefined)} />
              </Inline>
              <Inline gap="150" align="center" isWrapping>
                {#if next.find}<a class="act" href={next.find.href}>{t('app.modules.on_find', { place: t(next.find.code) })}</a>{/if}
                {#if next.setup}<a class="act" href={next.setup.href}>{t(next.setup.code)}</a>{/if}
              </Inline>
              {#if starts.length > 0}
                <Inline gap="150" align="center" isWrapping data-start-with>
                  <span class="quiet">{t('app.modules.on_start_with')}</span>
                  {#each starts as start (start.template.key)}
                    {#if start.href}
                      <a class="act" href={start.href} data-start-template={start.template.key}>{templateName(start.template)}</a>
                    {:else}
                      <Button tone="secondary" size="sm" data-start-template={start.template.key}
                        onclick={() => { startingFrom = start.template.key; isCreatingHub = true; }}>{templateName(start.template)}</Button>
                    {/if}
                  {/each}
                </Inline>
              {/if}
            </section>
          {/if}
        </div>
      {/each}
    </div>
    {#if hubId}
      <SectionFindings sections={['scheme', 'tolerance']} {hubId} />
    {:else}
      <ViewAsFile values={shown} {place} bind:isOpen={isFileOpen} {isTriggerHidden} />
    {/if}
  {/if}
</div>

{#if isCreatingHub}
  <CreateContainerDialog bind:isOpen={isCreatingHub} type="HUB" template={startingFrom} oncreated={created} />
{/if}

{#if confirming}
  <Dialog
    size="s"
    bind:isOpen={isConfirming}
    title={t('app.modules.off_title', { module: t(moduleCodes(confirming).name) })}
    dismissLabel={t('app.modules.off_cancel')}
    onClose={() => (confirming = undefined)}
    primary={{ label: t('app.modules.off_confirm'), tone: 'danger', onclick: confirmOff }}
  >
    <p class="quiet">{t('app.modules.off_explain')}</p>
    {#each reachLines as line (line.code)}<p class="quiet" data-off-reach>{t(line.code, line.params)}</p>{/each}
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isConfirming = false)}>{t('app.modules.off_cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .switches { display: flex; flex-direction: column; gap: var(--sp-200); min-width: 0; }

  .quiet { margin: 0; color: var(--text-secondary); }

  .waiting {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }

  .region {
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    overflow: clip;
  }

  /* The hairline between two modules; each row is wrapped with its note, so the design system's
     own rule between sibling rows does not see them. */
  .module + .module { border-block-start: var(--bw-hairline) solid var(--border-subtle); }

  .acts { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-150); }

  .refusal { margin: 0; padding: 0 var(--sp-250) var(--sp-150); color: var(--text-danger); }

  .card {
    display: grid;
    gap: var(--sp-100);
    padding: var(--sp-150) var(--sp-250);
    background: var(--accent-primary-subtle);
  }
  .card-title { margin: 0; font-weight: var(--fw-medium); }
  .act { color: var(--text-brand); font-weight: var(--fw-medium); }
</style>
