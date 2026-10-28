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

  import { Banner, Button, Dialog, Inline, Spinner, Stack, Switch } from '@hubtask/design-system/components';
  import type { ModuleList, ModuleState, ResourceState } from '@hubtask/sync-engine';

  import ValueSource from './ValueSource.svelte';
  import ViewAsFile from '../settings/ViewAsFile.svelte';
  import { asksFirst, isSetHere, moduleCodes, modules, unavailableBecause } from '../data/modules.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

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

  /** The module being switched, so its row shows progress. */
  let busy = $state<ModuleState['module'] | undefined>(undefined);
  /** The module whose switch-off is being asked about. */
  let confirming = $state<ModuleState | undefined>(undefined);
  let isConfirming = $state(false);
  let refusal = $state<{ module: ModuleState['module']; message: string } | undefined>(undefined);

  async function send(state: ModuleState, to: 'ON' | 'OFF' | 'INHERIT') {
    refusal = undefined;
    busy = state.module;
    try {
      await modules.switch(state.module, to, hubId);
      const name = t(moduleCodes(state).name);
      const code = to === 'ON' ? 'app.modules.switched_on' : to === 'OFF' ? 'app.modules.switched_off' : 'app.modules.reset_done';
      toast.show({ text: t(code, { module: name }), tone: 'success' });
    } catch (error) {
      refusal = { module: state.module, message: renderProblem(error as never, messages).message };
    } finally {
      busy = undefined;
    }
  }

  function change(state: ModuleState, event: Event) {
    // Kept before anything else: the read stays what is in force until the server answers.
    const control = event.currentTarget as HTMLInputElement;
    const next = control.checked;
    control.checked = state.on;
    if (asksFirst(state, next)) {
      confirming = state;
      isConfirming = true;
      return;
    }
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
    <Banner tone="danger" title={readRefusal.message}>
      {#if readRefusal.reference}{t('app.error_reference', { request_id: readRefusal.reference })}{/if}
    </Banner>
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
              checked={state.on}
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
          </Stack>
        </li>
      {/each}
    </ul>
    <Inline gap="150" align="center"><ViewAsFile values={shown} {place} /></Inline>
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
</style>
