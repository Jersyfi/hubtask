<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's sign-in providers under the ways in that switch them (UC-ID-11): this
  // workspace's own and the ones the installation offers, one `DataTable` region whose row opens
  // the provider's record page (page-system §9.2). It configures; whether a provider is on here is
  // said, and switched only in the list of ways above (UC-ID-11 check 8).
  //
  // An inherited row is not this workspace's to change, so its menu holds *Open* alone. Where the
  // plan leaves the workspace's own providers out they rest: none is added or changed, removing one
  // stays (multi-tenancy.md §4.2, P-05).
  import { untrack } from 'svelte';

  import { Badge, Button, DataTable, EmptyState, ReadOnlyNotice, Spinner, type DataColumn } from '@hubtask/design-system/components';
  import type { IdentityProvider } from '@hubtask/sync-engine';

  import ProviderDialog from './ProviderDialog.svelte';
  import ProviderMark from './ProviderMark.svelte';
  import RemoveProviderDialog from './RemoveProviderDialog.svelte';
  import { certificateState } from './certificate.ts';
  import { identityProvider } from '../data/identityprovider.svelte.ts';
  import { includedHere, mayHere } from '../data/capability.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import NotInPlan from '../workspace/NotInPlan.svelte';

  const { onopen }: { onopen: (path: string) => void } = $props();

  $effect(() => untrack(() => identityProvider.open()));

  const reading = $derived(identityProvider.state);
  const rows = $derived([...identityProvider.own, ...identityProvider.inherited]);
  /** `404`: an installation that does not serve providers - nothing to configure here. */
  const unavailable = $derived(reading.status === 'failed' && reading.error.status === 404);
  const unreadable = $derived(reading.status === 'failed' && !unavailable ? renderProblem(reading.error, messages) : undefined);
  const mayManage = $derived(mayHere('MANAGE_MEMBERS').status === 'permitted');
  const resting = $derived(includedHere('OWN_SIGN_IN_PROVIDERS') === false);

  let isAdding = $state(false);
  let editing = $state<IdentityProvider | undefined>(undefined);
  let isEditing = $state(false);
  let removing = $state<{ id: string; name: string } | undefined>(undefined);

  const addressOf = (provider: IdentityProvider): string => `/administration/sign-in/providers/${encodeURIComponent(provider.id)}`;
  const certificate = (provider: IdentityProvider) => certificateState(provider.saml_certificate_expires_at, Date.now());
  const certificateEnd = (provider: IdentityProvider): string =>
    provider.saml_certificate_expires_at ? formatDateTime(provider.saml_certificate_expires_at, messages.locale) : '';

  const columns: readonly DataColumn[] = $derived([
    { id: 'name', label: t('app.identity_provider.column_name'), priority: 1 },
    { id: 'issuer', label: t('app.identity_provider.issuer_label'), priority: 2 },
    { id: 'from', label: t('app.identity_provider.column_from'), priority: 3 },
    { id: 'state', label: t('app.identity_provider.column_state'), isStatus: true },
  ]);

  function menuOf(provider: IdentityProvider) {
    const isOwn = provider.scope === 'workspace';
    return {
      label: t('app.page.more_for', { name: provider.display_name }),
      items: [
        { id: 'open', label: t('app.page.open') },
        ...(isOwn && mayManage && !resting ? [{ id: 'edit', label: t('app.identity_provider.edit_more') }] : []),
        ...(isOwn && mayManage
          ? [{ id: 'remove', label: t('app.identity_provider.remove_more'), isDestructive: true, hasSeparatorBefore: true }]
          : []),
      ],
    };
  }

  function onmenu(provider: IdentityProvider, id: string): void {
    if (id === 'open') onopen(addressOf(provider));
    else if (id === 'edit') {
      editing = provider;
      isEditing = true;
    } else if (id === 'remove') removing = { id: provider.id, name: provider.display_name };
  }
</script>

<section class="providers" aria-labelledby="providers">
  <div class="head">
    <h3 id="providers">{t('app.identity_provider.title')}</h3>
    {#if reading.status === 'ready'}<span class="count">{rows.length}</span>{/if}
    {#if mayManage && !resting && !unavailable}
      <span class="acts"><Button icon="plus" onclick={() => (isAdding = true)}>{t('app.identity_provider.add')}</Button></span>
    {/if}
  </div>
  <p class="quiet">{t('app.identity_provider.intro')}</p>
  {#if resting}
    <NotInPlan body={t('app.plan.providers_body', { count: identityProvider.own.length, name: identityProvider.own[0]?.display_name ?? '' })} />
  {/if}

  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="quiet"><Spinner label={t('app.identity_provider.reading')} /> <span>{t('app.identity_provider.reading')}</span></p>
  {:else if unreadable}
    <FailureNote problem={unreadable} />
  {:else if unavailable}
    <ReadOnlyNotice icon="info" text={t('app.identity_provider.not_served')} />
  {:else}
    <DataTable
      label={t('app.identity_provider.title')}
      {columns}
      {rows}
      key={(provider: IdentityProvider) => provider.id}
      href={addressOf}
      rowLabel={(provider: IdentityProvider) => provider.display_name}
      menu={menuOf}
      {onmenu}
      actionsLabel={t('app.page.row_actions')}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      shownLabel={rows.length === 0 ? undefined : t('app.page.showing', { count: String(rows.length) })}
    >
      {#snippet cell(provider: IdentityProvider, column: DataColumn)}
        {#if column.id === 'name'}
          <span class="name"><ProviderMark kind={provider.kind} name={provider.display_name} />{provider.display_name}</span>
        {:else if column.id === 'issuer'}
          <span class="issuer">{provider.issuer}</span>
        {:else if column.id === 'from'}
          {provider.scope === 'workspace' ? t('app.identity_provider.from_workspace') : t('app.identity_provider.badge_inherited')}
        {:else if certificate(provider) === 'ended'}
          <Badge size="sm" tone="danger">{t('app.identity_provider.certificate_ended', { date: certificateEnd(provider) })}</Badge>
        {:else if certificate(provider) === 'ending'}
          <Badge size="sm" tone="warning">{t('app.identity_provider.certificate_ending', { date: certificateEnd(provider) })}</Badge>
        {:else if provider.scope === 'workspace' && resting}
          <Badge size="sm">{t('app.plan.resting')}</Badge>
        {:else if !provider.enabled}
          <Badge size="sm" tone="warning">{t('app.identity_provider.badge_off')}</Badge>
        {:else}
          <Badge size="sm" tone={provider.offered_here ? 'success' : 'neutral'}>
            {provider.offered_here ? t('app.identity_provider.badge_on_here') : t('app.identity_provider.badge_off_here')}
          </Badge>
        {/if}
      {/snippet}
      {#snippet empty()}
        <EmptyState kind="unused" title={t('app.identity_provider.none')} />
      {/snippet}
    </DataTable>
  {/if}
</section>

<ProviderDialog bind:isOpen={isAdding} />
<ProviderDialog provider={editing} bind:isOpen={isEditing} />
<RemoveProviderDialog target={removing} onClose={() => (removing = undefined)} />

<style>
  .providers { display: flex; flex-direction: column; gap: var(--sp-150); min-width: 0; }
  .head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  h3 { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  .count { color: var(--text-subtle); font-size: var(--fs-100); }
  .acts { margin-inline-start: auto; }
  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }
  .name { display: inline-flex; align-items: center; gap: var(--sp-100); color: var(--text-primary); font-weight: var(--fw-medium); }
  .issuer { overflow-wrap: anywhere; color: var(--text-secondary); }
</style>
