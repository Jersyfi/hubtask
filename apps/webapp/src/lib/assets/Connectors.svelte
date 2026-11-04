<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's connectors (UC-AST-07 check 7, UC-AST-08 check 7): the programs its CIs are pulled from
  // and the alert intakes monitoring programs post to, each with how its last run ended. An
  // administrator defines one and sets its credential; a member who may write runs a pull, as a
  // dry run first. An intake's address is shown to copy, its secret once, when it is drawn.
  //
  // **A credential is never read back** (security.md §8.1): the field is empty on every opening and
  // the connector only says whether one is set. A drawn secret lives in this component until the
  // reader confirms they kept it, and nowhere else.
  //
  // Each connector is one card: its name and program, how its last run ended in words, the one act
  // done most (*Run now*, or the intake's address to copy) and the rest in its menu
  // (user-experience.md §7.3). The head's create opens the dialog here (`isAsked`).

  import { untrack } from 'svelte';
  import {
    Badge, Banner, Button, Dialog, IconButton, Input, Menu, OneTimeSecret, Select, Skeleton, Stack, Switch, Textarea, type MenuItem, type SelectOption,
  } from '@hubtask/design-system/components';
  import type { CiClass, CiConnector, CiConnectorList, CiConnectorRun, CiSource, CiSourceList } from '@hubtask/sync-engine';

  import { holdsHere } from '../data/capability.svelte.ts';
  import {
    CONNECTOR_KINDS, CONNECTOR_PERIODS, cmdb, connectorDefine, connectorDraft, connectorMissing, connectorTone, connectorsPath, intakeAddress, kindSpec,
    sourcesPath, switchOn, type ConnectorDraft, type Words,
  } from '../data/cmdb.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import IntakeAddress from '../mail/IntakeAddress.svelte';
  import { renderProblem } from '../problem.ts';
  import ProductEmpty from '../product/ProductEmpty.svelte';
  import Decisions from './Decisions.svelte';

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
    mayWrite: boolean;
    /** The head asked for a new connector; the dialog opens once and `onasked` gives the ask back. */
    isAsked?: boolean;
    onasked?: () => void;
  }

  const { hubId, classes, words, mayWrite, isAsked = false, onasked }: Props = $props();

  const mayShape = $derived(holdsHere('STRUCTURE') === true);

  let connectors = $state<readonly CiConnector[] | undefined>(undefined);
  let sources = $state<readonly CiSource[]>([]);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const at = hubId;
    return untrack(() => {
      const stops = [
        engine.subscribe<CiConnectorList>({ path: connectorsPath(at) }, (state) => {
          if (state.status === 'ready') connectors = state.data.data;
          if (state.status === 'failed') refusal = renderProblem(state.error, messages).message;
        }),
        engine.subscribe<CiSourceList>({ path: sourcesPath }, (state) => {
          if (state.status === 'ready') sources = state.data.data.filter((each) => each.kind === 'CONNECTOR');
        }),
        containers.openLevel(at),
        workspace.open(),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });

  const fail = (error: unknown) => toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
  const when = (at: string | null | undefined) => (at ? formatDateTime(at, messages.locale) : '');
  const kindWord = (kind: CiConnector['kind']) => t(`app.assets.connector_kind.${kind}`);
  const settingWord = (key: string) => (messages.has(`app.assets.connector_setting.${key}`) ? t(`app.assets.connector_setting.${key}`) : key);
  const periodWord = (minutes: number) => t('app.assets.connector_every', { minutes });

  const kindOptions = $derived<SelectOption[]>(CONNECTOR_KINDS.map((each) => ({ value: each.kind, label: kindWord(each.kind) })));
  const sourceOptions = $derived<SelectOption[]>(sources.map((each) => ({ value: each.key, label: each.key })));
  const periodOptions = $derived<SelectOption[]>(CONNECTOR_PERIODS.map((each) => ({ value: String(each), label: periodWord(each) })));
  const collectionOptions = $derived<SelectOption[]>([
    { value: '', label: t('app.assets.connector_no_collection') },
    ...containers.collectionsOf(hubId).map((each) => ({ value: each.id, label: each.name })),
  ]);

  // Defining.
  let editing = $state<CiConnector | undefined>(undefined);
  let draft = $state<ConnectorDraft | undefined>(undefined);
  let period = $state('0');
  let isDefineOpen = $state(false);
  let defineFailure = $state<string | undefined>(undefined);
  const spec = $derived(draft ? kindSpec(draft.kind) : undefined);
  const missing = $derived(draft ? connectorMissing({ ...draft, every: Number(period) }, !editing) : []);
  const defineReason = $derived(missing.length ? t(missing[0]!, { key: spec?.settings.find((key) => !(draft?.settings[key] ?? '').trim()) ?? '' }) : undefined);

  function openDefine(of?: CiConnector) {
    editing = of;
    draft = connectorDraft(of);
    if (!of && sources[0]) draft.source = sources[0].key;
    period = String(draft.every);
    defineFailure = undefined;
    isDefineOpen = true;
  }

  $effect(() => {
    if (!isAsked || !mayShape) return;
    untrack(() => {
      openDefine();
      onasked?.();
    });
  });

  async function save() {
    if (!draft) return;
    try {
      await cmdb.defineConnector(hubId, editing?.key ?? draft.key, connectorDefine({ ...draft, every: Number(period) }), editing?.version);
      toast.show({ text: t('app.assets.connector_saved'), tone: 'success' });
      isDefineOpen = false;
    } catch (error) {
      // Said inside the dialog, which keeps what was typed (user-experience.md §5.10).
      defineFailure = renderProblem(error as never, messages).message;
    }
  }

  // The credential.
  let credentialFor = $state<CiConnector | undefined>(undefined);
  let credential = $state('');
  let isCredentialOpen = $state(false);
  let credentialFailure = $state<string | undefined>(undefined);
  let drawn = $state<{ readonly connector: CiConnector; readonly secret: string } | undefined>(undefined);

  function openCredential(connector: CiConnector) {
    credentialFor = connector;
    credential = '';
    credentialFailure = undefined;
    isCredentialOpen = true;
  }

  async function setCredential(connector: CiConnector, value?: string) {
    try {
      const answered = await cmdb.setConnectorCredential(connector, value);
      credential = '';
      isCredentialOpen = false;
      if (answered.secret) drawn = { connector: answered, secret: answered.secret };
      else toast.show({ text: t('app.assets.connector_credential_set'), tone: 'success' });
    } catch (error) {
      if (isCredentialOpen) credentialFailure = renderProblem(error as never, messages).message;
      else fail(error);
    }
  }

  // Running.
  let results = $state<Record<string, CiConnectorRun>>({});
  let busy = $state<string | undefined>(undefined);

  async function run(connector: CiConnector, options: { full?: boolean; dryRun?: boolean }) {
    busy = connector.id;
    try {
      const answer = await cmdb.runConnector(connector, options);
      results = { ...results, [connector.id]: answer };
      if (answer.outcome === 'FAILED') toast.show({ text: answer.error && messages.has(answer.error) ? t(answer.error) : t('app.assets.connector_failed'), tone: 'danger' });
      else toast.show({ text: options.dryRun ? t('app.assets.connector_tried') : t('app.assets.connector_ran'), tone: 'success' });
    } catch (error) {
      fail(error);
    } finally {
      busy = undefined;
    }
  }

  const counts = (answer: CiConnectorRun) => t('app.assets.run_counts', {
    created: answer.counts.created, updated: answer.counts.updated, unchanged: answer.counts.unchanged,
    held: answer.counts.held, duplicates: answer.counts.duplicates, refused: answer.counts.refused,
  });
  const lastWord = (connector: CiConnector) =>
    connector.last_outcome ? t(`app.assets.connector_outcome.${connector.last_outcome}`) : t('app.assets.connector_never');
  const errorWord = (code: string) => (messages.has(code) ? t(code) : code);
  /** The card's menu: everything but the act done most, the full run apart because it rereads everything. */
  function menuOf(connector: CiConnector): MenuItem[] {
    const reason = connector.has_credential ? undefined : t('connector.credential_missing');
    return [
      ...(mayWrite && !connector.push
        ? [
            { id: 'dry', label: t('app.assets.connector_dry_run'), icon: 'eye' as const, disabledReason: reason },
            { id: 'full', label: t('app.assets.connector_run_full'), icon: 'rotate-ccw' as const, disabledReason: reason },
          ]
        : []),
      ...(mayShape
        ? [
            { id: 'edit', label: t('app.assets.connector_edit'), icon: 'pencil' as const, hasSeparatorBefore: mayWrite && !connector.push },
            connector.push
              ? { id: 'secret', label: connector.has_credential ? t('app.assets.intake_redraw') : t('app.assets.intake_draw'), icon: 'key' as const }
              : { id: 'credential', label: t('app.assets.connector_credential'), icon: 'key' as const },
          ]
        : []),
    ];
  }
  function chose(connector: CiConnector, id: string) {
    if (id === 'dry') void run(connector, { dryRun: true });
    else if (id === 'full') void run(connector, { full: true });
    else if (id === 'edit') openDefine(connector);
    else if (id === 'secret') void setCredential(connector);
    else if (id === 'credential') openCredential(connector);
  }
  /** One sentence on how the pull stands: when it last ran, what failed, when it runs next. */
  function standing(connector: CiConnector): string {
    const parts = [
      connector.last_run_at ? t('app.assets.connector_last', { when: when(connector.last_run_at) }) : t('app.assets.connector_never'),
      ...(connector.failures > 0 ? [t('app.assets.connector_failures', { count: connector.failures })] : []),
      ...(connector.next_run_at && connector.enabled ? [t('app.assets.connector_next', { when: when(connector.next_run_at) })] : []),
    ];
    return parts.join(' · ');
  }

  const address = (connector: CiConnector) =>
    workspace.workspace?.id ? intakeAddress(window.location.origin, workspace.workspace.id, connector) : undefined;
</script>

<Stack gap="200">
  {#if drawn}
    <Stack gap="100">
      <OneTimeSecret
        value={drawn.secret}
        label={t('app.assets.intake_secret', { key: drawn.connector.key })}
        hint={t('app.assets.intake_secret_hint')}
        revealLabel={t('app.apps.reveal')}
        hideLabel={t('app.apps.hide')}
        copyLabel={t('app.apps.copy')}
        copiedLabel={t('app.apps.copied')}
        acknowledgementLabel={t('app.apps.kept')}
        notAcknowledgedReason={t('app.apps.keep_first')}
        dismissLabel={t('app.apps.done')}
        onDismiss={() => (drawn = undefined)}
      />
    </Stack>
  {/if}

  <div class="head">
    <p class="meta">{t('app.assets.connectors_intro')}</p>
    {#if mayShape && connectors?.length !== 0}<Button size="sm" tone="secondary" icon="plus" onclick={() => openDefine()}>{t('app.assets.connector_new')}</Button>{/if}
  </div>

  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if !connectors}
    <div aria-busy="true"><Skeleton lines={3} /></div>
  {:else if connectors.length === 0}
    <ProductEmpty product="ASSETS" place={{ type: 'HUB', id: hubId }}
      kind="unused"
      icon="link"
      title={t('app.assets.connectors_empty')}
      description={mayShape ? t('app.assets.connectors_empty_body') : t('app.assets.connectors_empty_reader')}
      create={mayShape ? { label: t('app.assets.connector_new'), onclick: () => openDefine() } : undefined}
    />
  {:else}
    <ul class="rows">
      {#each connectors as connector (connector.id)}
        {@const answer = results[connector.id]}
        {@const items = menuOf(connector)}
        {@const reason = connector.has_credential ? undefined : t('connector.credential_missing')}
        <li class="connector" data-connector={connector.key} data-off={connector.enabled ? undefined : ''}>
          <div class="row-line">
            <span class="what">
              <strong class="name">{connector.key}</strong>
              <span class="meta">
                {kindWord(connector.kind)} · {t('app.assets.connector_writes_as', { source: connector.source })}
                {#if !connector.push} · {periodWord(connector.every_minutes)}{/if}
              </span>
            </span>
            <span class="state">
              {#if !connector.enabled}<Badge size="sm" icon="pause">{t('app.assets.connector_off')}</Badge>{/if}
              {#if !connector.has_credential}<Badge size="sm" tone="warning">{t('app.assets.connector_no_credential')}</Badge>{/if}
              {#if !connector.push}<Badge size="sm" tone={connectorTone(connector.last_outcome)}>{lastWord(connector)}</Badge>{/if}
              {#if items.length > 0}
                <Menu label={t('app.assets.connector_menu', { key: connector.key })} {items} onselect={(id) => chose(connector, id)}>
                  {#snippet trigger(props)}
                    <IconButton {...props} icon="ellipsis" size="sm" label={t('app.assets.connector_menu', { key: connector.key })} />
                  {/snippet}
                </Menu>
              {/if}
            </span>
          </div>

          {#if !connector.push}
            <p class="meta">{standing(connector)}</p>
            {#if connector.last_error}<p class="meta">{errorWord(connector.last_error)}</p>{/if}
            {#if connector.base_url}<p class="meta mono">{connector.base_url}</p>{/if}
          {:else}
            {@const at = address(connector)}
            {#if at}<IntakeAddress address={at} lead={connector.inventory ? t('app.assets.intake_agent_address') : t('app.assets.intake_address')} hint={t('app.assets.intake_address_hint')} />{/if}
          {/if}

          {#if mayWrite && !connector.push}
            <div class="actions">
              <Button size="sm" tone="secondary" icon="play" disabledReason={reason} isBusy={busy === connector.id} onclick={() => void run(connector, {})}>{t('app.assets.connector_run')}</Button>
            </div>
          {/if}

          {#if answer}
            <div class="answer" data-connector-answer>
              <p class="meta">
                {answer.dry_run ? t('app.assets.connector_dry_answer', { pages: answer.pages }) : t('app.assets.connector_answer', { pages: answer.pages })}
                · {counts(answer)}
              </p>
              {#if answer.outcome === 'FAILED' && answer.error}<Banner tone="danger">{errorWord(answer.error)}</Banner>{/if}
              {#if answer.decisions?.length}<Decisions decisions={answer.decisions} {classes} {words} />{/if}
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</Stack>

{#if isDefineOpen && draft}
  <Dialog bind:isOpen={isDefineOpen} title={editing ? t('app.cmdb_admin.edit_named', { name: editing.key }) : t('app.assets.connector_new')} dismissLabel={t('app.assets.cancel')}>
    <Stack gap="150">
      {#if !editing}
        <Input label={t('app.cmdb_admin.key')} hint={t('app.cmdb_admin.key_hint')} bind:value={draft.key} maxlength={50} />
        <Select label={t('app.assets.connector_kind_label')} options={kindOptions} bind:value={draft.kind} />
      {/if}
      {#if sourceOptions.length === 0}
        <Banner tone="info">{t('app.assets.connector_source_missing')}</Banner>
      {:else}
        <Select label={t('app.assets.connector_source_label')} hint={t('app.assets.connector_source_hint')} options={sourceOptions} bind:value={draft.source} />
      {/if}
      {#if spec && !spec.push}
        <Input label={t('app.assets.connector_base_url')} hint={t('app.assets.connector_base_url_hint')} inputmode="url" bind:value={draft.baseUrl} maxlength={2000} />
      {/if}
      {#each spec?.settings ?? [] as key (key)}
        <Input label={settingWord(key)} bind:value={() => draft?.settings[key] ?? '', (typed: string) => draft && (draft.settings[key] = typed)} maxlength={500} />
      {/each}
      {#each spec?.switches ?? [] as each (each.key)}
        <Switch
          label={t(`app.assets.connector_switch.${each.key}`)}
          bind:checked={() => !!draft && switchOn(draft, each), (on) => draft && (draft.settings[each.key] = on ? each.on : each.off)}
        />
      {/each}
      {#if spec && !spec.push}
        <Textarea label={t('app.assets.connector_hosts')} hint={t('app.assets.connector_hosts_hint')} rows={2} bind:value={draft.hosts} />
        <Select label={t('app.assets.connector_period')} options={periodOptions} bind:value={period} />
      {:else if spec && !spec.inventory}
        <Select label={t('app.assets.connector_collection')} hint={t('app.assets.connector_collection_hint')} options={collectionOptions} bind:value={draft.collection} />
      {/if}
      <Switch label={t('app.assets.connector_enabled')} bind:checked={draft.enabled} />
      {#if defineFailure}<Banner tone="danger">{defineFailure}</Banner>{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isDefineOpen = false)}>{t('app.assets.cancel')}</Button>
      <Button tone="primary" disabledReason={defineReason} onclick={() => void save()}>{t('app.assets.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if isCredentialOpen && credentialFor}
  <Dialog bind:isOpen={isCredentialOpen} title={t('app.assets.connector_credential_title', { key: credentialFor.key })} dismissLabel={t('app.assets.cancel')}>
    <Stack gap="150">
      <Input
        label={t('app.assets.connector_credential_label')}
        hint={t('app.assets.connector_credential_hint')}
        type="password"
        autocomplete="new-password"
        bind:value={credential}
        maxlength={4096}
      />
      {#if credentialFailure}<Banner tone="danger">{credentialFailure}</Banner>{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isCredentialOpen = false)}>{t('app.assets.cancel')}</Button>
      <Button
        tone="primary"
        disabledReason={credential ? undefined : t('cmdb.connector_credential_invalid')}
        onclick={() => credentialFor && void setCredential(credentialFor, credential)}
      >{t('app.assets.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-100); }
  .row-line { display: flex; flex-wrap: wrap; align-items: start; justify-content: space-between; gap: var(--sp-100); }
  .connector {
    display: grid; gap: var(--sp-100);
    padding: var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .connector[data-off] { background: var(--bg-surface-sunken); }
  .what, .answer { display: grid; gap: var(--sp-025); min-inline-size: 0; }
  .name { overflow-wrap: break-word; }
  .state { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050); }
  .actions { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
  .mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
</style>
