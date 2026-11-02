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

  import { Button, Dialog, IconButton, Inline, Spinner, Stack, Switch } from '@hubtask/design-system/components';
  import type { ModuleList, ModuleReach, ModuleState, ResourceState } from '@hubtask/sync-engine';

  import ValueSource from './ValueSource.svelte';
  import ViewAsFile from '../settings/ViewAsFile.svelte';
  import SectionFindings from '../settings/SectionFindings.svelte';
  import ViewSectionsAsFile from '../settings/ViewSectionsAsFile.svelte';
  import { asksFirst, isSetHere, moduleCodes, modules, offReach, unavailableBecause } from '../data/modules.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { newMarks } from '../product/newmark.svelte.ts';
  import { switchedOn } from '../product/switchedon.ts';

  interface Props {
    /** The hub to switch for; absent is the workspace. */
    hubId?: string;
  }

  const { hubId }: Props = $props();
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

<Stack gap="300">
  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="waiting">
      <Spinner label={t('app.modules.reading')} />
      <span>{t('app.modules.reading')}</span>
    </p>
  {:else if readRefusal}
    <FailureNote problem={readRefusal} />
  {:else}
    <ul class="modules">
      {#each all as state (state.module)}
        {@const codes = moduleCodes(state)}
        {@const reason = unavailableBecause(state, place)}
        <li class="panel" data-module={state.module} data-setting={`module.${state.module.toLowerCase()}`}>
          <Stack gap="150">
            <Switch
              label={t(codes.name)}
              hint={t(codes.hint)}
              bind:checked={() => drawn[state.module] ?? state.on, (value) => (drawn[state.module] = value)}
              onchange={(event) => change(state, event)}
              isBusy={busy === state.module}
              busyLabel={t('app.workspace.saving')}
              disabledReason={reason ? t(reason) : undefined}
            />
            <Inline gap="150" align="center">
              <ValueSource
                source={state.source}
                lock={state.locked_by}
                atHub={place === 'HUB'}
                value={state.on ? t('app.signin_settings.on') : t('app.signin_settings.off')}
              />
              {#if isSetHere(state, place)}
                <Button tone="subtle" size="sm" data-reset={state.module} onclick={() => send(state, 'INHERIT')}>
                  {t('app.modules.reset')}
                </Button>
              {/if}
            </Inline>
            {#if refusal?.module === state.module}<p class="refusal" role="alert">{refusal.message}</p>{/if}
            {#if justOn === state.module && state.on}
              {@const next = switchedOn(state.module, hubId)}
              <!-- The same card for every module: where it is now, and what it still needs. -->
              <section class="card" aria-label={t('app.modules.on_card', { module: t(codes.name) })} data-switched-on={state.module}>
                <Inline justify="between" align="center">
                  <p class="card-title">{t('app.modules.on_card', { module: t(codes.name) })}</p>
                  <IconButton icon="x" size="sm" label={t('app.modules.on_card_close')} onclick={() => (justOn = undefined)} />
                </Inline>
                <Inline gap="150" align="center" isWrapping>
                  {#if next.find}<a class="act" href={next.find.href}>{t('app.modules.on_find', { place: t(next.find.code) })}</a>{/if}
                  {#if next.setup}<a class="act" href={next.setup.href}>{t(next.setup.code)}</a>{/if}
                </Inline>
              </section>
            {/if}
          </Stack>
        </li>
      {/each}
    </ul>
    <Inline gap="150" align="center">
      {#if hubId}
        <ViewSectionsAsFile sections={['modules', 'structure', 'scheme', 'business_calendars']} {hubId} />
      {:else}
        <ViewAsFile values={shown} {place} />
      {/if}
    </Inline>
    {#if hubId}<SectionFindings sections={['scheme', 'tolerance']} {hubId} />{/if}
  {/if}
</Stack>

{#if confirming}
  <Dialog
    bind:isOpen={isConfirming}
    title={t('app.modules.off_title', { module: t(moduleCodes(confirming).name) })}
    dismissLabel={t('app.modules.off_cancel')}
    onClose={() => (confirming = undefined)}
  >
    <p class="quiet">{t('app.modules.off_explain')}</p>
    {#each reachLines as line (line.code)}<p class="quiet" data-off-reach>{t(line.code, line.params)}</p>{/each}
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isConfirming = false)}>{t('app.modules.off_cancel')}</Button>
      <Button tone="danger" data-confirm-off onclick={confirmOff}>{t('app.modules.off_confirm')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  .waiting {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }

  .modules {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: var(--sp-200);
  }

  .panel {
    padding: var(--sp-250);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .refusal { margin: 0; color: var(--text-danger); }

  .card {
    display: grid;
    gap: var(--sp-100);
    padding: var(--sp-150) var(--sp-200);
    border-radius: var(--r-md);
    background: var(--accent-primary-subtle);
  }
  .card-title { margin: 0; font-weight: var(--fw-medium); }
  .act { color: var(--text-brand); font-weight: var(--fw-medium); }
</style>
