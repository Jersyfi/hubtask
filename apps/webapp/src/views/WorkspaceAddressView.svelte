<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's address (UC-INS-19 check 3, multi-tenancy.md §3.1): the main address, and who may
  // change it. Three states, from what the server answers rather than from the reader's role:
  //
  // - the installation leaves the change to its operator - the default: no control, and why;
  // - it lets owners change it and the reader is one: *Change address*, the operator's dialog with its
  //   costs, the lockout rule and the proof by password or provider;
  // - a change is inside the installation's cooldown: no control, and the day from which it is
  //   possible again in its place.
  //
  // After the owner's change the old address answers nothing and every session there ended, so the
  // browser goes on to the new address's front door.

  import { untrack } from 'svelte';

  import { Banner, Button, PageHeader, Spinner, Stack } from '@hubtask/design-system/components';

  import ChangeAddressDialog from '../lib/address/ChangeAddressDialog.svelte';
  import OwnDomainPanel from '../lib/address/OwnDomainPanel.svelte';
  import { addressAfterChange, type AddressSubject } from '../lib/address/change.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatLongDay } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  $effect(() => untrack(() => workspace.open()));
  $effect(() => page.entitle(t('app.address.title')));

  const reading = $derived(workspace.state);
  const current = $derived(workspace.workspace);
  const address = $derived(current?.address);
  const refusal = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);

  let changing = $state<AddressSubject | undefined>(undefined);

  const day = (iso: string | null | undefined): string => (iso ? formatLongDay(iso, messages.locale) : '');

  /** What the screen says beneath the address: why there is no control, or what the control costs. */
  const note = $derived.by(() => {
    if (!address || !address.owners_may_change) return t('app.address.operator_only');
    if (address.may_change) return t('app.address.owners_change');
    if (address.changeable_from) {
      return t('app.address.cooldown', { changed: day(address.changed_at), from: day(address.changeable_from) });
    }
    return t('app.address.owners_only');
  });

  function open(): void {
    if (!current || !address) return;
    changing = { id: current.id, name: current.display_name, slug: current.slug, host: address.host };
  }
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.address.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'address', label: t('app.address.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="quiet">
      <Spinner label={t('app.address.reading')} />
      <span>{t('app.address.reading')}</span>
    </p>
  {:else if refusal}
    <Banner tone="danger" title={refusal.message}>
      {#if refusal.reference}{t('app.error_reference', { request_id: refusal.reference })}{/if}
    </Banner>
  {:else if current}
    <section class="panel" aria-labelledby="main-address">
      <Stack gap="100">
        <span class="label" id="main-address">{t('app.address.main')}</span>
        <p class="fixed">{address?.host ?? current.slug}</p>
        <p class="note">{note}</p>
        {#if address?.may_change}
          <div><Button tone="secondary" onclick={open}>{t('app.address.change')}</Button></div>
        {/if}
      </Stack>
    </section>
    <!-- UC-INS-15 check 1: where the installation allows own domains; one set up before it stopped
         allowing them is still shown, for its owner to see and remove. -->
    {#if address && (address.own_domains_allowed || address.own_domain)}
      <OwnDomainPanel
        own={address.own_domain}
        main={address.host}
        isOwner={address.may_manage_own_domain === true}
        isAllowed={address.own_domains_allowed === true}
      />
    {/if}
  {/if}
</Stack>

<ChangeAddressDialog
  subject={changing}
  isAssigned={manifest.hasAssignedHosts}
  change={(body) => workspace.changeAddress(body)}
  onclose={() => (changing = undefined)}
  onchanged={(answer) => {
    if (answer.to) location.assign(addressAfterChange(answer.to, location));
  }}
/>

<style>
  .quiet {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }

  /* The surface the address stands on, as the section's other screens draw one, with the measure
     of the prose inside it. */
  .panel {
    max-inline-size: 72ch;
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }

  .label { color: var(--text-primary); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .fixed {
    margin: 0;
    font-family: var(--font-mono);
    font-size: var(--fs-200);
    color: var(--text-primary);
    overflow-wrap: anywhere;
  }

  /* The panel carries the measure, so the note runs its width as the preview draws it. */
  .note { margin: 0; max-inline-size: none; font-size: var(--fs-075); color: var(--text-subtle); }
</style>
