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
  //
  // Where the own domain is the main address (UC-INS-15 checks 7 and 8) the panel names it, and the
  // subdomain beside it that sends browsers on, with the way back; the own domain's own panel is then
  // this one. Either move of the main address ends every session, and the browser goes on likewise.
  //
  // A `SettingsLayout` of one section (page-system §9.1); a change of address opens its Dialog M,
  // which names the plan - the new address and what the change ends - before anything moves.

  import { untrack } from 'svelte';

  import { Badge, Button, SettingRow, SettingsLayout, SettingsSection, Spinner } from '@hubtask/design-system/components';

  import ChangeAddressDialog from '../lib/address/ChangeAddressDialog.svelte';
  import MainAddressDialog from '../lib/address/MainAddressDialog.svelte';
  import OwnDomainPanel from '../lib/address/OwnDomainPanel.svelte';
  import { addressAfterChange, type AddressSubject } from '../lib/address/change.ts';
  import type { MainAddressMove } from '../lib/address/owndomain.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { formatLongDay } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import FailureNote from '../lib/frame/FailureNote.svelte';

  interface Props {
    /** Opens another address, entering it into the history: the way back below `expanded`. */
    onnavigate?: (path: string) => void;
  }
  const { onnavigate }: Props = $props();

  const title = $derived(t('app.address.title'));
  $effect(() => untrack(() => workspace.open()));
  $effect(() => page.entitle(title));

  const reading = $derived(workspace.state);
  const current = $derived(workspace.workspace);
  const address = $derived(current?.address);
  const refusal = $derived(reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined);

  let changing = $state<AddressSubject | undefined>(undefined);
  let moving = $state<MainAddressMove | undefined>(undefined);

  const own = $derived(address?.own_domain ?? undefined);
  /** The own domain is the main address, with the subdomain beside it as the way back. */
  const wayBack = $derived(own?.is_main && own.state !== 'BLOCKED' ? (address?.address_host ?? undefined) : undefined);

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

<SettingsLayout
  head={{
    title,
    breadcrumb: {
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'address', label: title },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    },
  }}
  nav={{ label: t('app.settings_page.sections', { name: title }), sections: [{ id: 'address', label: title, href: '/administration/address' }], current: 'address' }}
  view="detail"
  back={{ label: t('app.admin.title'), onback: () => onnavigate?.('/administration') }}
>
  {#if reading.status === 'loading' || reading.status === 'idle'}
    <p class="quiet">
      <Spinner label={t('app.address.reading')} />
      <span>{t('app.address.reading')}</span>
    </p>
  {:else if refusal}
    <FailureNote problem={refusal} />
  {:else if current}
    <!-- One section (page-system §9.1): the main address, the way back where the own domain is
         main, and the own domain, as rows of one region. -->
    <SettingsSection title={title}>
      {#if address && wayBack}
        <div data-main="own">
        <div class="row">
          <SettingRow label={t('app.address.main')} help={t('app.address.own_main_note')} fieldWidth="lg">
            {#snippet source()}<Badge tone="success">{t('app.address.own_main_badge')}</Badge>{/snippet}
            {#snippet control()}<p class="fixed">{address.host}</p>{/snippet}
          </SettingRow>
        </div>
        <div class="row">
          <SettingRow label={t('app.address.also')} help={t('app.address.also_note', { main: address.host })} fieldWidth="lg">
            {#snippet control()}
              <p class="fixed">{wayBack}</p>
              {#if address.may_manage_own_domain}
                <Button tone="secondary" size="sm" onclick={() => (moving = { direction: 'back', from: address.host, to: wayBack })}>
                  {t('app.address.switch_back', { host: wayBack })}
                </Button>
              {/if}
            {/snippet}
          </SettingRow>
        </div>
        </div>
      {:else}
        <div class="row">
          <SettingRow label={t('app.address.main')} help={note} fieldWidth="lg">
            {#snippet control()}
              <p class="fixed">{address?.host ?? current.slug}</p>
              {#if address?.may_change}
                <Button tone="secondary" size="sm" onclick={open}>{t('app.address.change')}</Button>
              {/if}
            {/snippet}
          </SettingRow>
        </div>
        <!-- UC-INS-15 check 1: where the installation allows own domains; one set up before it
             stopped allowing them is still shown, for its owner to see and remove. -->
        {#if address && (address.own_domains_allowed || address.own_domain)}
          <div class="row own">
            <OwnDomainPanel
              own={address.own_domain}
              main={address.host}
              isOwner={address.may_manage_own_domain === true}
              isAllowed={address.own_domains_allowed === true}
              onmakemain={() => {
                if (address.own_domain) moving = { direction: 'own', from: address.host, to: address.own_domain.domain };
              }}
            />
          </div>
        {/if}
      {/if}
    </SettingsSection>
  {/if}
</SettingsLayout>

<ChangeAddressDialog
  subject={changing}
  isAssigned={manifest.hasAssignedHosts}
  change={(body) => workspace.changeAddress(body)}
  onclose={() => (changing = undefined)}
  onchanged={(answer) => {
    if (answer.to) location.assign(addressAfterChange(answer.to, location));
  }}
/>

<MainAddressDialog
  move={moving}
  send={(body) => workspace.moveMainAddress(moving?.direction ?? 'own', body)}
  onclose={() => (moving = undefined)}
  onmoved={(answer) => {
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

  .fixed {
    margin: 0;
    font-family: var(--font-mono);
    font-size: var(--fs-200);
    color: var(--text-primary);
    overflow-wrap: anywhere;
  }

  /* The rows are wrapped so a test finds each with its parts; the hairline between two rows is
     drawn here, since the design system's own rule sees siblings. */
  .row + .row :global(.setting-row > .setting-line:first-child),
  .row + .own { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
</style>
