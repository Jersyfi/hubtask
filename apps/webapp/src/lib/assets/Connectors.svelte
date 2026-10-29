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

  import { untrack } from 'svelte';
  import {
    Badge, Banner, Button, Dialog, EmptyState, Input, OneTimeSecret, Select, Spinner, Stack, Switch, Textarea, type SelectOption,
  } from '@hubtask/design-system/components';
  import type { CiClass, CiConnector, CiConnectorList, CiConnectorRun, CiSource, CiSourceList } from '@hubtask/sync-engine';

  import { holdsHere } from '../data/capability.svelte.ts';
  import {
    CONNECTOR_KINDS, CONNECTOR_PERIODS, cmdb, connectorDefine, connectorDraft, connectorMissing, connectorTone, connectorsPath, intakeAddress, kindSpec,
    sourcesPath, type ConnectorDraft, type Words,
  } from '../data/cmdb.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import IntakeAddress from '../mail/IntakeAddress.svelte';
  import { renderProblem } from '../problem.ts';
  import Decisions from './Decisions.svelte';

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
    mayWrite: boolean;
  }

  const { hubId, classes, words, mayWrite }: Props = $props();

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
  const spec = $derived(draft ? kindSpec(draft.kind) : undefined);
  const missing = $derived(draft ? connectorMissing({ ...draft, every: Number(period) }, !editing) : []);
  const defineReason = $derived(missing.length ? t(missing[0]!, { key: spec?.settings.find((key) => !(draft?.settings[key] ?? '').trim()) ?? '' }) : undefined);

  function openDefine(of?: CiConnector) {
    editing = of;
    draft = connectorDraft(of);
    if (!of && sources[0]) draft.source = sources[0].key;
    period = String(draft.every);
    isDefineOpen = true;
  }

  async function save() {
    if (!draft) return;
    try {
      await cmdb.defineConnector(hubId, editing?.key ?? draft.key, connectorDefine({ ...draft, every: Number(period) }), editing?.version);
      toast.show({ text: t('app.assets.connector_saved'), tone: 'success' });
      isDefineOpen = false;
    } catch (error) {
      fail(error);
    }
  }

  // The credential.
  let credentialFor = $state<CiConnector | undefined>(undefined);
  let credential = $state('');
  let isCredentialOpen = $state(false);
  let drawn = $state<{ readonly connector: CiConnector; readonly secret: string } | undefined>(undefined);

  function openCredential(connector: CiConnector) {
    credentialFor = connector;
    credential = '';
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
      fail(error);
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
  const address = (connector: CiConnector) =>
    workspace.workspace?.id ? intakeAddress(window.location.origin, workspace.workspace.id, connector.id) : undefined;
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
    {#if mayShape}<Button size="sm" tone="primary" icon="plus" onclick={() => openDefine()}>{t('app.assets.connector_new')}</Button>{/if}
  </div>

  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if !connectors}
    <Spinner label={t('app.assets.loading')} />
  {:else if connectors.length === 0}
    <EmptyState kind="unused" title={t('app.assets.connectors_empty')} icon="link" />
  {:else}
    <ul class="rows">
      {#each connectors as connector (connector.id)}
        {@const answer = results[connector.id]}
        <li class="connector" data-connector={connector.key}>
          <div class="row-line">
            <span class="what">
              <strong>{connector.key}</strong>
              <span class="meta">
                {kindWord(connector.kind)} · {t('app.assets.connector_writes_as', { source: connector.source })}
                {#if !connector.push} · {periodWord(connector.every_minutes)}{/if}
                {#if !connector.enabled} · {t('app.assets.connector_off')}{/if}
              </span>
              {#if !connector.push && connector.base_url}<span class="meta mono">{connector.base_url}</span>{/if}
              <span class="meta">{connector.has_credential ? t('app.assets.connector_has_credential') : t('app.assets.connector_no_credential')}</span>
            </span>
            {#if !connector.push}
              <span class="outcome">
                <Badge size="sm" tone={connectorTone(connector.last_outcome)}>{lastWord(connector)}</Badge>
                {#if connector.last_run_at}<span class="meta">{when(connector.last_run_at)}</span>{/if}
                {#if connector.last_error}<span class="meta">{errorWord(connector.last_error)}</span>{/if}
                {#if connector.failures > 0}<span class="meta">{t('app.assets.connector_failures', { count: connector.failures })}</span>{/if}
                {#if connector.next_run_at && connector.enabled}<span class="meta">{t('app.assets.connector_next', { when: when(connector.next_run_at) })}</span>{/if}
              </span>
            {/if}
          </div>

          {#if connector.push}
            {@const at = address(connector)}
            {#if at}<IntakeAddress address={at} lead={t('app.assets.intake_address')} />{/if}
          {/if}

          <div class="actions">
            {#if mayShape}
              <Button size="sm" tone="subtle" icon="pencil" onclick={() => openDefine(connector)}>{t('app.assets.connector_edit')}</Button>
              {#if connector.push}
                <Button size="sm" tone="subtle" icon="key" onclick={() => void setCredential(connector)}>
                  {connector.has_credential ? t('app.assets.intake_redraw') : t('app.assets.intake_draw')}
                </Button>
              {:else}
                <Button size="sm" tone="subtle" icon="key" onclick={() => openCredential(connector)}>{t('app.assets.connector_credential')}</Button>
              {/if}
            {/if}
            {#if mayWrite && !connector.push}
              {@const reason = connector.has_credential ? undefined : t('connector.credential_missing')}
              <Button size="sm" tone="secondary" disabledReason={reason} isBusy={busy === connector.id} onclick={() => void run(connector, { dryRun: true })}>{t('app.assets.connector_dry_run')}</Button>
              <Button size="sm" tone="secondary" disabledReason={reason} isBusy={busy === connector.id} onclick={() => void run(connector, {})}>{t('app.assets.connector_run')}</Button>
              <Button size="sm" tone="subtle" disabledReason={reason} isBusy={busy === connector.id} onclick={() => void run(connector, { full: true })}>{t('app.assets.connector_run_full')}</Button>
            {/if}
          </div>

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
        <Input label={settingWord(key)} bind:value={draft.settings[key]} maxlength={500} />
      {/each}
      {#if spec && !spec.push}
        <Textarea label={t('app.assets.connector_hosts')} hint={t('app.assets.connector_hosts_hint')} rows={2} bind:value={draft.hosts} />
        <Select label={t('app.assets.connector_period')} options={periodOptions} bind:value={period} />
      {:else}
        <Select label={t('app.assets.connector_collection')} hint={t('app.assets.connector_collection_hint')} options={collectionOptions} bind:value={draft.collection} />
      {/if}
      <Switch label={t('app.assets.connector_enabled')} bind:checked={draft.enabled} />
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isDefineOpen = false)}>{t('app.assets.cancel')}</Button>
      <Button tone="primary" disabledReason={defineReason} onclick={() => void save()}>{t('app.assets.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if isCredentialOpen && credentialFor}
  <Dialog bind:isOpen={isCredentialOpen} title={t('app.assets.connector_credential_title', { key: credentialFor.key })} dismissLabel={t('app.assets.cancel')}>
    <Input
      label={t('app.assets.connector_credential_label')}
      hint={t('app.assets.connector_credential_hint')}
      type="password"
      autocomplete="new-password"
      bind:value={credential}
      maxlength={4096}
    />
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
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row-line { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: var(--sp-100); }
  .connector {
    display: grid; gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .what, .outcome, .answer { display: grid; gap: var(--sp-025); }
  .outcome { justify-items: end; }
  .actions { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
  .mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
</style>
