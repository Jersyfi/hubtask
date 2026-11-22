<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The module switches of one place (UC-INS-21): the workspace under *How we work → Modules*, or
  // one hub in its settings. One switch per module, with where its state comes from and whether a
  // level above locked it; a hub decides for itself inside what the workspace has on (check 3).
  //
  // **A switch acts at once**, as every single switch of the administration does - one row is not
  // a form. Switching off asks first and says that the data is kept and hidden (check 4). Switching
  // on needs no configuration step (check 2); at the workspace it asks one thing where hubs exist -
  // in which of them (check 13) - because it changes no hub by itself.
  //
  // **What a level above locked is not offered** (check 6): the control is switched off with the
  // reason, and the chip says whose value it is and whom to ask. A value set here offers *Reset*,
  // which hands the module back to the level above.

  import { untrack } from 'svelte';

  import { Button, CheckboxGroup, Dialog, Icon, IconButton, Inline, Radio, SettingRow, Spinner, Switch } from '@hubtask/design-system/components';
  import type { ConfigTemplate, ConfigTemplateList, ModuleList, ModuleReach, ModuleState, ResourceState } from '@hubtask/sync-engine';

  import { buckets } from '../data/buckets.svelte.ts';
  import { workflows } from '../data/workflows.svelte.ts';
  import { statusLabel } from '../data/workflows.ts';
  import { needsChoice, rowsInReach, strandedBy, type OffChoice, type RowReach, type SchemeRow } from './workflowsoff.ts';

  import ValueSource from './ValueSource.svelte';
  import ViewAsFile from '../settings/ViewAsFile.svelte';
  import SectionFindings from '../settings/SectionFindings.svelte';
  import { asksFirst, asksWhichHubs, hubsOffered, isOfferSettled, isSetHere, moduleCodes, modules, offReach, unavailableBecause } from '../data/modules.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { newMarks } from '../product/newmark.svelte.ts';
  import { settingsOf, startsOf, switchedOn } from '../product/switchedon.ts';
  import { holds, holdsHere, hubCreationHere, roleOrder } from '../data/capability.svelte.ts';
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

  // Switching Workflows off (UC-INS-21 check 14): a workflow stays in force on the
  // server with its module off, so the question lists what it strands - read while the module is
  // still on: the scheme, one preview of removing each row in reach, each board's columns - and
  // asks whether to convert those places to columns or only pause; nothing is chosen at first.
  const asksWorkflows = $derived(confirming?.module === 'WORKFLOWS');
  $effect(() => {
    if (!asksWorkflows) return;
    const at = hubId;
    return untrack(() => {
      const stops = [workflows.open()];
      if (at) stops.push(containers.openLevel(at));
      return () => stops.forEach((stop) => stop());
    });
  });
  const inReach = $derived.by((): SchemeRow[] | undefined => {
    const list = workflows.list;
    if (!asksWorkflows || !list) return undefined;
    if (hubId && containers.isLevelLoading(hubId)) return undefined;
    return rowsInReach(list.scheme, hubId, hubId ? containers.collectionsOf(hubId).map((each) => each.id) : []);
  });
  let reaches = $state<
    | { status: 'idle' | 'loading' }
    | { status: 'ready'; data: readonly RowReach[] }
    | { status: 'failed'; error: unknown }
  >({ status: 'idle' });
  $effect(() => {
    const rows = inReach;
    reaches = { status: rows ? 'loading' : 'idle' };
    if (!rows) return;
    let isCurrent = true;
    void Promise.all(rows.map(async (row) => ({
      row,
      preview: await workflows.previewScheme({ container_id: row.container_id, ...(row.entry_type ? { entry_type: row.entry_type } : {}) }),
    }))).then(
      (data) => { if (isCurrent) reaches = { status: 'ready', data }; },
      (error) => { if (isCurrent) reaches = { status: 'failed', error }; },
    );
    return () => { isCurrent = false; };
  });
  const reached = $derived(reaches.status === 'ready' ? reaches.data : undefined);
  // The places and boards named in the question: their names and the columns that show each status.
  $effect(() => {
    const ids = [...new Set([...(reached ?? []).map((each) => each.row.container_id), ...(reached ?? []).flatMap((each) => each.preview.moves.map((move) => move.collection_id))])];
    if (ids.length === 0) return;
    return untrack(() => {
      const stops = ids.flatMap((id) => [containers.openSingle(id), buckets.open(id)]);
      return () => stops.forEach((stop) => stop());
    });
  });
  const stranded = $derived(reached ? strandedBy(reached, workflows.list?.data ?? [], (id) => buckets.of(id)) : undefined);
  let offChoice = $state<OffChoice>(undefined);
  /** Converting: the places done so far, the one at, and why it stopped. */
  let converting = $state<{ done: number; at?: string; failure?: string } | undefined>(undefined);

  /** A place as the question names it: a collection with its hub, a hub with all its collections. */
  function placeName(id: string): string {
    const place = containers.find(id);
    if (!place) return t('app.modules.off_wf_unknown');
    if (place.type === 'HUB') return t('app.modules.off_wf_hub_place', { hub: place.name });
    const hub = place.parent_id ? containers.find(place.parent_id) : undefined;
    return hub ? t('app.modules.off_wf_collection_place', { collection: place.name, hub: hub.name }) : place.name;
  }
  function statusName(collectionId: string, key: string): string {
    const row = reached?.find((each) => each.preview.moves.some((move) => move.collection_id === collectionId));
    const shape = workflows.list?.data.find((each) => each.key === row?.row.workflow)?.published;
    const status = shape?.statuses.find((each) => each.key === key);
    return status ? statusLabel(status, t) : key;
  }
  function columnAfter(collectionId: string, columnId: string | undefined): string {
    const columns = buckets.of(collectionId);
    const column = columns.find((each) => each.id === columnId);
    if (column) return column.name;
    return columns[0] ? t('app.modules.off_wf_first_column', { column: columns[0].name }) : t('app.modules.off_wf_no_board');
  }

  /** Removes each scheme row in reach, then switches off; a failure stops and keeps the module on. */
  async function convertAndSwitchOff(state: ModuleState) {
    const rows = reached?.map((each) => each.row) ?? [];
    converting = { done: 0 };
    for (const row of rows) {
      converting = { done: converting.done, at: placeName(row.container_id) };
      try {
        await workflows.setScheme({ container_id: row.container_id, ...(row.entry_type ? { entry_type: row.entry_type } : {}) });
      } catch (error) {
        converting = { done: converting.done, failure: renderProblem(error as never, messages).message };
        return;
      }
      converting = { done: converting.done + 1 };
    }
    converting = undefined;
    isConfirming = false;
    confirming = undefined;
    await send(state, 'OFF');
    toast.show({ text: t('app.modules.off_wf_converted', { count: rows.length }), tone: 'success' });
  }
  const offPrimary = $derived.by(() => {
    if (!asksWorkflows) return { label: t('app.modules.off_confirm'), tone: 'danger' as const, onclick: confirmOff };
    if (reaches.status === 'loading' || reaches.status === 'idle') {
      return { label: t('app.modules.off_confirm'), tone: 'danger' as const, onclick: confirmOff, disabledReason: t('app.modules.off_wf_reading') };
    }
    if (!needsChoice(stranded)) return { label: t('app.modules.off_confirm'), tone: 'danger' as const, onclick: confirmOff };
    if (offChoice === 'convert') {
      return {
        label: t('app.modules.off_wf_convert_confirm'), tone: 'danger' as const,
        onclick: () => { if (confirming) void convertAndSwitchOff(confirming); },
        isBusy: converting !== undefined && !converting.failure, busyLabel: t('app.modules.off_wf_converting_short'),
      };
    }
    if (offChoice === 'pause') return { label: t('app.modules.off_wf_pause_confirm'), tone: 'danger' as const, onclick: confirmOff };
    return { label: t('app.modules.off_confirm'), tone: 'danger' as const, onclick: confirmOff, disabledReason: t('app.modules.off_wf_choose_first') };
  });

  // A private hub counts only a role held on it, so its memberships are read; a shared hub
  // follows the workspace's, which the manifest already answers.
  $effect(() => {
    if (hubId) return;
    const hidden = containers.hubs.filter((hub) => hub.private === true).map((hub) => hub.id);
    untrack(() => hidden.forEach((id) => people.open({ hubId: id })));
  });
  /** Whether the reader may shape a private hub, by the role held on it; `undefined` while that is out. */
  const shapesPrivate = (hub: { readonly id: string }): boolean | undefined => {
    const role = people.roleOf({ hubId: hub.id }, actor.account?.id, roleOrder());
    if (role === undefined) return undefined;
    return role !== null && holds(role, 'STRUCTURE').status === 'permitted';
  };
  // The hubs the switch-on's question offers: those the reader may shape (check 13) among the hub
  // list, which is one page - the first 200 hubs that are not archived (containers.svelte.ts). A
  // hub beyond it is not offered here and is switched on in its own settings
  // (user-experience.md §2.8).
  const offered = $derived(hubId ? [] : hubsOffered(containers.hubs, holdsHere('STRUCTURE'), shapesPrivate));
  /**
   * Whether the offer can be decided: the hub list and every hub's answer are in. A list that
   * failed is decided with no hub - the switch then changes none, as with none chosen.
   */
  const isSettled = $derived.by(() => {
    const list = containers.hubsState.status;
    if (list === 'failed') return true;
    return isOfferSettled(list === 'ready' ? containers.hubs : undefined, holdsHere('STRUCTURE'), shapesPrivate);
  });
  /** The module whose switch-on waits for the offer to be decided. */
  let waiting = $state<ModuleState | undefined>(undefined);
  $effect(() => {
    const state = waiting;
    if (!state || !isSettled) return;
    untrack(() => {
      waiting = undefined;
      busy = undefined;
      switchOn(state);
    });
  });
  /** The module whose switch-on is asking in which existing hubs, and the hubs ticked: none at first. */
  let asking = $state<ModuleState | undefined>(undefined);
  let isAsking = $state(false);
  let chosen = $state<readonly string[]>([]);

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

  async function send(state: ModuleState, to: 'ON' | 'OFF' | 'INHERIT', inHubs: readonly string[] = []) {
    refusal = undefined;
    busy = state.module;
    try {
      await modules.switch(state.module, to, hubId, inHubs);
      justOn = to === 'ON' ? state.module : justOn === state.module ? undefined : justOn;
      if (to === 'ON' && switchedOn(state.module, hubId).find) newMarks.add(state.module);
      const name = t(moduleCodes(state).name);
      const code = to === 'OFF' ? 'app.modules.switched_off' : to === 'INHERIT' ? 'app.modules.reset_done'
        : inHubs.length > 0 ? 'app.modules.switched_on_hubs' : 'app.modules.switched_on';
      toast.show({ text: t(code, { module: name, count: inHubs.length }), tone: 'success' });
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
      offChoice = undefined;
      converting = undefined;
      confirming = state;
      isConfirming = true;
      return;
    }
    if (next && place === 'WORKSPACE' && !isSettled) {
      // Not decided on the hubs read so far: with none offered yet it would go through unasked.
      control.checked = state.on;
      drawn[state.module] = state.on;
      busy = state.module;
      waiting = state;
      return;
    }
    if (asksWhichHubs(state, next, place, offered)) {
      control.checked = state.on;
      drawn[state.module] = state.on;
      ask(state);
      return;
    }
    drawn[state.module] = next;
    void send(state, next ? 'ON' : 'OFF');
  }

  function ask(state: ModuleState) {
    chosen = [];
    asking = state;
    isAsking = true;
  }

  /** The switch-on that waited: after the question where hubs are offered (check 13), at once where none is (check 2). */
  function switchOn(state: ModuleState) {
    if (asksWhichHubs(state, true, place, offered)) return ask(state);
    drawn[state.module] = true;
    void send(state, 'ON');
  }

  function confirmOn() {
    const state = asking;
    // A hub that left the list while the question stood open is not sent.
    const inHubs = chosen.filter((id) => offered.some((hub) => hub.id === id));
    isAsking = false;
    asking = undefined;
    if (!state) return;
    drawn[state.module] = true;
    void send(state, 'ON', inHubs);
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
        {@const reason = unavailableBecause(state, place, manifest.value?.modules_on)}
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
    size={asksWorkflows && needsChoice(stranded) ? 'm' : 's'}
    bind:isOpen={isConfirming}
    title={t('app.modules.off_title', { module: t(moduleCodes(confirming).name) })}
    dismissLabel={t('app.modules.off_cancel')}
    onClose={() => (confirming = undefined)}
    primary={offPrimary}
  >
    {#if asksWorkflows}
      <!-- What switching off strands, as the alert dialog's list of consequences (UC-INS-21 check 14). -->
      <p class="quiet">{t('app.modules.off_wf_explain')}</p>
      {#if reaches.status === 'loading'}
        <p class="waiting"><Spinner label={t('app.modules.off_wf_reading')} /><span>{t('app.modules.off_wf_reading')}</span></p>
      {:else if reaches.status === 'failed'}
        <FailureNote problem={renderProblem(reaches.error as never, messages)} />
      {/if}
      <ul class="consequences" data-off-workflows>
        {#if stranded && stranded.places.length > 0}
          <li data-tone="warning"><Icon name="triangle-alert" size="sm" /><span><strong>{t('app.modules.off_wf_places', { count: stranded.places.length })}</strong><small>{stranded.places.map(placeName).join(' · ')}</small></span></li>
        {/if}
        {#if stranded && stranded.open > 0}
          <li data-tone="warning" data-off-open={stranded.open}><Icon name="triangle-alert" size="sm" /><span><strong>{t('app.modules.off_wf_open', { count: stranded.open })}</strong>{#if stranded.stuck > 0}<small data-off-stuck={stranded.stuck}>{t('app.modules.off_wf_stuck', { count: stranded.stuck })}</small>{/if}</span></li>
        {/if}
        {#each reachLines as line (line.code)}<li data-off-reach><Icon name={line.code === 'app.modules.off_rules' ? 'pause' : 'info'} size="sm" /><span>{t(line.code, line.params)}</span></li>{/each}
        <li><Icon name="info" size="sm" /><span>{t('app.modules.off_wf_kept')}</span></li>
      </ul>
      {#if stranded && needsChoice(stranded)}
        <Radio
          label={t('app.modules.off_wf_question', { count: stranded.open })}
          options={[
            { value: 'convert', label: t('app.modules.off_wf_convert'), hint: t('app.modules.off_wf_convert_hint') },
            { value: 'pause', label: t('app.modules.off_wf_pause'), hint: t('app.modules.off_wf_pause_hint', { open: stranded.open, stuck: stranded.stuck }) },
          ]}
          bind:value={() => offChoice ?? '', (value) => (offChoice = value === 'convert' || value === 'pause' ? value : undefined)}
          disabledReason={converting && !converting.failure ? t('app.modules.off_wf_converting_short') : undefined}
        />
        {#if offChoice === 'convert'}
          <!-- Where the entries go, per collection: each status today and the column that keeps it. -->
          <div class="where" data-off-where>
            <p class="where-title">{t('app.modules.off_wf_where')}</p>
            {#each [...new Set(stranded.lines.map((line) => line.collectionId))] as collectionId (collectionId)}
              <p class="where-place">{containers.find(collectionId)?.name ?? t('app.modules.off_wf_unknown')}</p>
              <ul>
                {#each stranded.lines.filter((line) => line.collectionId === collectionId) as line (line.status)}
                  <li data-off-line>{t('app.modules.off_wf_line', { status: statusName(collectionId, line.status), column: columnAfter(collectionId, line.columnId), count: line.count })}</li>
                {/each}
              </ul>
            {/each}
          </div>
        {/if}
        {#if converting}
          <p class="quiet" role="status" data-converting>
            {#if converting.failure}{t('app.modules.off_wf_stopped', { done: converting.done, count: stranded.places.length })}{:else if converting.at}{t('app.modules.off_wf_converting', { at: converting.done + 1, count: stranded.places.length, place: converting.at })}{/if}
          </p>
          {#if converting.failure}<p class="refusal-inline" role="alert">{converting.failure}</p>{/if}
        {/if}
      {/if}
    {:else}
      <p class="quiet">{t('app.modules.off_explain')}</p>
      {#each reachLines as line (line.code)}<p class="quiet" data-off-reach>{t(line.code, line.params)}</p>{/each}
    {/if}
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isConfirming = false)}>{t('app.modules.off_cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if asking}
  <Dialog
    size="m"
    bind:isOpen={isAsking}
    title={t('app.modules.on_hubs_title', { module: t(moduleCodes(asking).name) })}
    dismissLabel={t('app.modules.on_hubs_cancel')}
    onClose={() => (asking = undefined)}
    primary={{ label: t('app.modules.on_hubs_confirm'), onclick: confirmOn }}
  >
    <div data-on-hubs={asking.module}>
      <CheckboxGroup
        label={t('app.modules.on_hubs_question')}
        hint={t('app.modules.on_hubs_hint', { module: t(moduleCodes(asking).name) })}
        options={offered.map((hub) => ({ value: hub.id, label: hub.name }))}
        selected={chosen}
        onChange={(next) => (chosen = next)}
      />
    </div>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isAsking = false)}>{t('app.modules.on_hubs_cancel')}</Button>
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

  /* The consequences of switching Workflows off, one line each with its mark (UX-7: never by colour alone). */
  .consequences { display: grid; gap: var(--sp-100); margin: var(--sp-150) 0; padding: 0; list-style: none; }
  .consequences li { display: grid; grid-template-columns: auto 1fr; gap: var(--sp-100); align-items: start; color: var(--text-secondary); }
  .consequences li[data-tone='warning'] {
    padding: var(--sp-100);
    border-radius: var(--r-sm);
    background: var(--status-warning-surface);
    color: var(--status-warning-text);
  }
  .consequences li[data-tone='warning'] small { color: inherit; }
  .consequences strong { display: block; font-weight: var(--fw-semibold); }
  .consequences small { display: block; color: var(--text-secondary); font-size: var(--fs-075); }
  .where { display: grid; gap: var(--sp-050); margin-block-start: var(--sp-150); font-size: var(--fs-075); }
  .where p { margin: 0; }
  .where-title { font-weight: var(--fw-medium); }
  .where-place { color: var(--text-secondary); }
  .where ul { margin: 0; padding-inline-start: var(--sp-200); }
  .refusal-inline { margin: 0; color: var(--text-danger); }
  .act { color: var(--text-brand); font-weight: var(--fw-medium); }
</style>
