<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The *Directory* tab of the sign-in screen (UC-ID-31): the LDAP directory the workspace's people
  // and groups are synchronised from, the last run, and the run on demand.
  //
  // **What is refused is said, never predicted.** The server binds before it stores and refuses a
  // run that would empty the workspace; this panel shows the refusal and the count it names, and
  // the one way through - giving the count back - is a button the person presses knowing it.
  //
  // **The password goes one way**, as a provider's secret does: an empty field on a directory that
  // exists keeps the sealed one, and the hint says so.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, Input, Select, Spinner, Stack, Switch } from '@hubtask/design-system/components';
  import type { LdapDirectory, LdapDirectoryConfiguration, LdapSyncRun } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import { ldapDirectory } from '../data/ldapdirectory.svelte.ts';
  import { serviceAccounts } from '../data/serviceaccounts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import WorkspaceGate from '../workspace/WorkspaceGate.svelte';

  type Draft = {
    url: string;
    bindDn: string;
    password: string;
    userBase: string;
    userFilter: string;
    groupBase: string;
    groupFilter: string;
    idAttribute: string;
    mailAttribute: string;
    nameAttribute: string;
    runAs: string;
    everyMinutes: string;
    enabled: boolean;
  };

  function draftOf(d: LdapDirectory | undefined): Draft {
    return {
      url: d?.url ?? '',
      bindDn: d?.bind_dn ?? '',
      password: '',
      userBase: d?.user_base ?? '',
      userFilter: d?.user_filter ?? '',
      groupBase: d?.group_base ?? '',
      groupFilter: d?.group_filter ?? '',
      idAttribute: d?.id_attribute ?? '',
      mailAttribute: d?.mail_attribute ?? '',
      nameAttribute: d?.name_attribute ?? '',
      runAs: d?.run_as ?? '',
      everyMinutes: String(d?.every_minutes ?? 60),
      enabled: d?.enabled ?? true,
    };
  }

  let draft = $state<Draft>(draftOf(undefined));
  let filled = $state(false);
  let isWorking = $state(false);
  let confirmingRemoval = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let saved = $state(false);
  let run = $state<LdapSyncRun | undefined>(undefined);

  $effect(() => untrack(() => ldapDirectory.open()));
  $effect(() => untrack(() => serviceAccounts.open()));

  const directory = $derived(ldapDirectory.directory);
  const reading = $derived(ldapDirectory.state);
  const unreadable = $derived(
    reading.status === 'failed' && !ldapDirectory.none ? renderProblem(reading.error, messages) : undefined,
  );

  // Filled once from what the server holds, never again while the person types.
  $effect(() => {
    if (filled) return;
    if (directory || ldapDirectory.none) {
      draft = draftOf(directory);
      filled = true;
    }
  });

  const accountOptions = $derived([
    { value: '', label: t('app.ldap.run_as_choose') },
    ...serviceAccounts.all
      .filter((account) => account.status === 'ACTIVE')
      .map((account) => ({ value: account.id, label: account.display_name })),
  ]);

  function problemOf(cause: unknown): ReturnType<typeof renderProblem> {
    return cause instanceof TransportError
      ? renderProblem(cause, messages)
      : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
  }

  function bodyOf(d: Draft): LdapDirectoryConfiguration {
    const optional = (value: string): string | undefined => (value.trim() === '' ? undefined : value.trim());
    return {
      url: d.url.trim(),
      bind_dn: d.bindDn.trim(),
      ...(d.password === '' ? {} : { bind_password: d.password }),
      user_base: d.userBase.trim(),
      user_filter: optional(d.userFilter),
      group_base: optional(d.groupBase),
      group_filter: optional(d.groupFilter),
      id_attribute: optional(d.idAttribute),
      mail_attribute: optional(d.mailAttribute),
      name_attribute: optional(d.nameAttribute),
      run_as: d.runAs,
      every_minutes: Number.parseInt(d.everyMinutes, 10) || 60,
      enabled: d.enabled,
    };
  }

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    isWorking = true;
    failure = undefined;
    saved = false;
    try {
      const stored = await ldapDirectory.configure(bodyOf(draft));
      // Out of the component's state at once: a password kept for a second save is a password
      // sitting in a form for as long as the tab is open.
      draft = { ...draftOf(stored) };
      saved = true;
    } catch (cause) {
      failure = problemOf(cause);
    } finally {
      isWorking = false;
    }
  }

  async function remove(): Promise<void> {
    isWorking = true;
    failure = undefined;
    try {
      await ldapDirectory.remove();
      confirmingRemoval = false;
      draft = draftOf(undefined);
      run = undefined;
    } catch (cause) {
      failure = problemOf(cause);
    } finally {
      isWorking = false;
    }
  }

  async function sync(dryRun: boolean, confirm?: number): Promise<void> {
    isWorking = true;
    failure = undefined;
    try {
      run = await ldapDirectory.sync(dryRun, confirm);
    } catch (cause) {
      failure = problemOf(cause);
    } finally {
      isWorking = false;
    }
  }

  const COUNTS = [
    'seen', 'provisioned', 'linked', 'renamed', 'switched_off', 'switched_on', 'kept', 'skipped', 'failed',
    'groups_created', 'groups_changed',
  ] as const;

  function tone(outcome: string | undefined): 'success' | 'warning' | 'danger' | 'neutral' {
    if (outcome === 'OK') return 'success';
    if (outcome === 'PARTIAL') return 'warning';
    if (outcome === 'REFUSED' || outcome === 'FAILED') return 'danger';
    return 'neutral';
  }
</script>

<Stack gap="300">
  <p class="quiet">{t('app.ldap.lead')}</p>

  {#if reading.status === 'idle' || reading.status === 'loading'}
    <p class="quiet"><Spinner size="sm" /> <span>{t('app.workspace.reading')}</span></p>
  {:else if unreadable}
    <Banner tone="danger" title={unreadable.message} />
  {:else}
    {#if failure}
      <Banner tone="danger" title={failure.message}>
        {#if failure.reference}{failure.reference}{/if}
      </Banner>
    {/if}
    {#if saved}
      <Banner tone="success" title={t('app.ldap.saved')} />
    {/if}

    {#if directory}
      <section class="panel">
        <Stack gap="150">
          <h2 class="section">{t('app.ldap.last_run')}</h2>
          {#if directory.last_run_at}
            <p class="row">
              <Badge tone={tone(directory.last_outcome)}>{t(`app.ldap.outcome.${(directory.last_outcome ?? 'OK').toLowerCase()}`)}</Badge>
              <span class="quiet">{formatDateTime(directory.last_run_at, messages.locale)}</span>
            </p>
            {#if directory.last_error}
              <p class="quiet">{messages.t(directory.last_error, {})}</p>
            {/if}
            {#if directory.last_counts}
              <dl class="counts">
                {#each COUNTS as key (key)}
                  <div><dt>{t(`app.ldap.count.${key}`)}</dt><dd>{directory.last_counts[key]}</dd></div>
                {/each}
              </dl>
            {/if}
          {:else}
            <p class="quiet">{t('app.ldap.never_ran')}</p>
          {/if}
          {#if directory.next_run_at && directory.enabled}
            <p class="quiet">{t('app.ldap.next_run', { at: formatDateTime(directory.next_run_at, messages.locale) })}</p>
          {/if}
          <WorkspaceGate permission="MANAGE_MEMBERS">
            {#snippet actions()}
              <div class="row">
                <Button tone="primary" isBusy={isWorking} onclick={() => sync(false)}>{t('app.ldap.run_now')}</Button>
                <Button tone="subtle" onclick={() => sync(true)}>{t('app.ldap.dry_run')}</Button>
              </div>
            {/snippet}
          </WorkspaceGate>
          {#if run}
            <Banner
              tone={run.outcome === 'OK' ? 'success' : run.outcome === 'PARTIAL' ? 'warning' : 'danger'}
              title={run.dry_run ? t('app.ldap.dry_run_result') : t(`app.ldap.outcome.${run.outcome.toLowerCase()}`)}
            >
              {#if run.error}{messages.t(run.error, { count: String(run.switch_off_planned), limit: String(run.switch_off_limit) })}{' '}{/if}
              {t('app.ldap.run_summary', {
                provisioned: run.counts.provisioned,
                switched_off: run.counts.switched_off,
                switched_on: run.counts.switched_on,
                failed: run.counts.failed,
              })}
            </Banner>
            {#if run.error === 'directory.ldap_switch_off_too_many'}
              <!-- The guard's one way through: the count, given back by the person who read it. -->
              <div class="row">
                <Button tone="danger" isBusy={isWorking} onclick={() => sync(false, run?.switch_off_planned)}>
                  {t('app.ldap.confirm_switch_off', { count: run.switch_off_planned })}
                </Button>
              </div>
            {/if}
          {/if}
        </Stack>
      </section>
    {/if}

    <WorkspaceGate permission="MANAGE_MEMBERS">
      {#snippet actions()}
        <form class="panel" onsubmit={save}>
          <Stack gap="200">
            <h2 class="section">{directory ? t('app.ldap.edit_title') : t('app.ldap.add_title')}</h2>
            <Input label={t('app.ldap.url_label')} hint={t('app.ldap.url_hint')} bind:value={draft.url} autocomplete="off" spellcheck={false} isRequired error={failure?.fields.get('/url')} />
            <Input label={t('app.ldap.bind_dn_label')} bind:value={draft.bindDn} autocomplete="off" spellcheck={false} isRequired error={failure?.fields.get('/bind_dn')} />
            <Input
              label={t('app.ldap.password_label')}
              hint={directory ? t('app.ldap.password_kept') : undefined}
              bind:value={draft.password}
              type="password"
              autocomplete="off"
              spellcheck={false}
              isRequired={!directory}
              error={failure?.fields.get('/bind_password')}
            />
            <Input label={t('app.ldap.user_base_label')} bind:value={draft.userBase} autocomplete="off" spellcheck={false} isRequired error={failure?.fields.get('/user_base')} />
            <Input label={t('app.ldap.user_filter_label')} hint={t('app.ldap.user_filter_hint')} bind:value={draft.userFilter} autocomplete="off" spellcheck={false} error={failure?.fields.get('/user_filter')} />
            <Input label={t('app.ldap.group_base_label')} hint={t('app.ldap.group_base_hint')} bind:value={draft.groupBase} autocomplete="off" spellcheck={false} error={failure?.fields.get('/group_base')} />
            <Input label={t('app.ldap.group_filter_label')} bind:value={draft.groupFilter} autocomplete="off" spellcheck={false} error={failure?.fields.get('/group_filter')} />
            <Input label={t('app.ldap.id_attribute_label')} hint={t('app.ldap.id_attribute_hint')} bind:value={draft.idAttribute} autocomplete="off" spellcheck={false} error={failure?.fields.get('/id_attribute')} />
            <Input label={t('app.ldap.mail_attribute_label')} bind:value={draft.mailAttribute} autocomplete="off" spellcheck={false} />
            <Input label={t('app.ldap.name_attribute_label')} bind:value={draft.nameAttribute} autocomplete="off" spellcheck={false} />
            <Select label={t('app.ldap.run_as_label')} hint={t('app.ldap.run_as_hint')} options={accountOptions} bind:value={draft.runAs} error={failure?.fields.get('/run_as')} />
            <Input label={t('app.ldap.every_label')} bind:value={draft.everyMinutes} type="number" min="15" max="1440" error={failure?.fields.get('/every_minutes')} />
            <Switch
              label={t('app.ldap.enabled_label')}
              checked={draft.enabled}
              onchange={(event) => (draft.enabled = (event.currentTarget as HTMLInputElement).checked)}
            />
            <div class="row">
              <Button type="submit" tone="primary" isBusy={isWorking} busyLabel={t('app.ldap.saving')}>{t('app.ldap.save')}</Button>
              {#if directory && !confirmingRemoval}
                <Button tone="subtle" onclick={() => (confirmingRemoval = true)}>{t('app.ldap.remove')}</Button>
              {/if}
            </div>
            {#if confirmingRemoval}
              <div class="confirm">
                <p>{t('app.ldap.remove_consequence')}</p>
                <div class="row">
                  <Button tone="danger" isBusy={isWorking} onclick={remove}>{t('app.ldap.remove_confirm')}</Button>
                  <Button tone="subtle" onclick={() => (confirmingRemoval = false)}>{t('app.ldap.keep')}</Button>
                </div>
              </div>
            {/if}
          </Stack>
        </form>
      {/snippet}
    </WorkspaceGate>
  {/if}
</Stack>

<style>
  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .row { margin: 0; display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .confirm { display: grid; gap: var(--sp-100); }

  form { margin: 0; max-inline-size: 52ch; }

  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }

  .counts {
    margin: 0;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(16ch, 1fr));
    gap: var(--sp-100);
  }

  .counts div { display: grid; gap: var(--sp-025); }
  .counts dt { color: var(--text-secondary); font-size: var(--fs-100); }
  .counts dd { margin: 0; font-weight: var(--fw-medium); }
</style>
