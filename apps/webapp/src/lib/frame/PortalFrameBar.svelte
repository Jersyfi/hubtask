<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The outsiders' frame (PO-2, E-13, scene `portalext`): whoever holds READ nowhere gets their
  // portal's bar alone - its mark and name, chosen at the top where there are several, the tabs
  // *start*, *my requests* and *approvals* with their counts (PO-3) and *knowledge*, the language
  // and the account. No sidebar, no Today, no search over Hubtask and no *Erstellen*. On a phone
  // the four tabs stand at the foot, and the languages head the account's sheet behind the bar's
  // menu mark. Only the web app's frame changes: every door refuses an
  // outsider as before.

  import type { Snippet } from 'svelte';
  import { untrack } from 'svelte';
  import { BottomBar, Icon, Menu, NavItem } from '@hubtask/design-system/components';
  import type { ApprovalList, PortalRequestPage, RequestCatalogue } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { myApprovalsPath } from '../data/approvals.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { cataloguePath, myRequestsPath } from '../data/portal.svelte.ts';
  import { localesOf } from '../data/preferences.ts';
  import { preferences } from '../data/preferences.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { languageName } from '../i18n/locale.ts';
  import { lastPortal } from '../portal/lastportal.svelte.ts';
  import { portalTabOf, portalTabPath, type PortalTab } from '../portal/portal.ts';
  import type { Resolution } from '../router.ts';
  import ContainerMark from '../workspace/ContainerMark.svelte';

  interface Props {
    route: Resolution;
    isPhone: boolean;
    onnavigate: (path: string) => void;
    /** The account's own menu, as the frame draws it everywhere; on a phone with rows of the bar's own. */
    account: Snippet<[extra?: Snippet<[close: () => void]>]>;
  }
  const { route, isPhone, onnavigate, account }: Props = $props();

  const portals = $derived(actor.portals);
  // The portal in view: the address's own, else the one chosen last, else the first.
  const routeId = $derived(route.name !== 'portal-request' && route.name?.startsWith('portal') ? route.params.id : undefined);
  const current = $derived(
    portals.find((each) => each.collection_id === routeId)
      ?? portals.find((each) => each.collection_id === lastPortal.id)
      ?? portals[0],
  );
  const currentId = $derived(current?.collection_id ?? '');

  // The counts beside the tabs: open requests of the first page here, approvals waiting anywhere.
  type Asked = { id: string; catalogue: ReturnType<typeof resource<RequestCatalogue>>; mine: ReturnType<typeof resource<PortalRequestPage>> };
  let asked = $state<Asked | undefined>(undefined);
  $effect(() => {
    const id = currentId;
    if (id === '') return;
    untrack(() => {
      if (asked?.id !== id) asked = { id, catalogue: resource<RequestCatalogue>(cataloguePath(id)), mine: resource<PortalRequestPage>(myRequestsPath(id)) };
    });
  });
  const approvals = untrack(() => resource<ApprovalList>(myApprovalsPath));
  const openCount = $derived(asked?.mine.state.status === 'ready' ? asked.mine.state.data.data.filter((each) => !each.is_completed).length : 0);
  const waitingCount = $derived(approvals.state.status === 'ready' ? approvals.state.data.data.length : 0);
  const desk = $derived(asked?.catalogue.state.status === 'ready' ? asked.catalogue.state.data.desk : undefined);
  const name = $derived(desk?.name || current?.name || '');

  const tab = $derived(portalTabOf(route.name));
  const tabs = $derived<{ id: PortalTab; label: string; count?: number; icon: 'house' | 'inbox' | 'user-check' | 'book-open' }[]>([
    { id: 'start', label: t('app.portal_frame.start'), icon: 'house' },
    { id: 'requests', label: t('app.portal_frame.requests'), count: openCount, icon: 'inbox' },
    { id: 'approvals', label: t('app.portal_frame.approvals'), count: waitingCount, icon: 'user-check' },
    { id: 'knowledge', label: t('app.portal_frame.knowledge'), icon: 'book-open' },
  ]);
  const labelOf = (each: { label: string; count?: number }) => (each.count ? `${each.label} · ${each.count}` : each.label);

  const locales = $derived(localesOf(manifest.value));
  const language = $derived(actor.account?.locale || messages.locale);
  async function speak(locale: string) {
    const id = actor.account?.id;
    if (!id || locale === language) return;
    await preferences.setAccount(id, { locale });
  }
  function choosePortal(id: string) {
    lastPortal.remember(id);
    onnavigate(portalTabPath(id, tab ?? 'start'));
  }
</script>

<header class="bar" data-portal-frame aria-label={t('app.portal_frame.label')}>
  <div class="lead">
    <a class="home" href={portalTabPath(currentId, 'start')} onclick={(event) => { event.preventDefault(); onnavigate(portalTabPath(currentId, 'start')); }}>
      <ContainerMark look={{ type: 'COLLECTION', icon: desk?.icon || null, color_token: desk?.color_token || null }} size="md" />
      {#if portals.length <= 1}<span class="name">{t('app.portal_frame.name', { name })}</span>{/if}
    </a>
    {#if portals.length > 1}
      <!-- Several portals are chosen at the top (PO-2). -->
      <Menu label={t('app.portal_frame.choose')} items={portals.map((each) => ({ id: each.collection_id, label: each.name, icon: each.collection_id === currentId ? 'check' : undefined }))}
        onselect={choosePortal}>
        {#snippet trigger(props)}
          <button type="button" class="switch" data-portal-switch {...props}>
            <span class="name">{t('app.portal_frame.name', { name })}</span><Icon name="chevron-down" size="sm" />
          </button>
        {/snippet}
      </Menu>
    {/if}
    {#if !isPhone}
      <nav class="tabs" aria-label={t('app.portal_frame.tabs')}>
        {#each tabs as each (each.id)}
          {@const path = portalTabPath(currentId, each.id)}
          <a class="tab" href={path} data-portal-tab={each.id} aria-current={tab === each.id ? 'page' : undefined}
            onclick={(event) => { event.preventDefault(); onnavigate(path); }}>{labelOf(each)}</a>
        {/each}
      </nav>
    {/if}
  </div>
  <div class="end">
    {#if locales.length > 1 && !isPhone}
      <Menu label={t('app.portal_frame.language')} items={locales.map((each) => ({ id: each.locale, label: languageName(each.locale, messages.locale), icon: each.locale === language ? 'check' : undefined }))}
        onselect={(locale) => void speak(locale)}>
        {#snippet trigger(props)}
          <button type="button" class="switch quiet" data-portal-language {...props}>{languageName(language, messages.locale)}<Icon name="chevron-down" size="sm" /></button>
        {/snippet}
      </Menu>
    {/if}
    {@render account(isPhone && locales.length > 1 ? languageRows : undefined)}
  </div>
</header>

{#snippet languageRows(close: () => void)}
  <h3 class="group-label">{t('app.portal_frame.language')}</h3>
  <ul class="rows" data-portal-language>
    {#each locales as each (each.locale)}
      <NavItem label={languageName(each.locale, messages.locale)} icon={each.locale === language ? 'check' : 'globe'} isCurrent={each.locale === language} size="touch"
        onnavigate={() => { close(); void speak(each.locale); }} />
    {/each}
  </ul>
{/snippet}

{#if isPhone}
  <BottomBar label={t('app.portal_frame.tabs')} current={tab}
    destinations={tabs.map((each) => ({ id: each.id, label: each.label, icon: each.icon, href: portalTabPath(currentId, each.id), count: each.count || undefined }))}
    onnavigate={(id) => onnavigate(portalTabPath(currentId, id as PortalTab))} />
{/if}

<style>
  .bar {
    display: flex; align-items: center; justify-content: space-between; gap: var(--sp-200);
    min-block-size: var(--layout-appbar-height); padding-inline: var(--sp-300); background: var(--bg-surface);
    border-block-end: var(--bw-hairline) solid var(--border-subtle); position: sticky; inset-block-start: 0; z-index: 2;
  }
  .lead, .end { display: flex; align-items: center; gap: var(--sp-200); min-inline-size: 0; }
  .home { display: inline-flex; align-items: center; gap: var(--sp-150); color: inherit; text-decoration: none; }
  .name { font-weight: var(--fw-bold); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .switch {
    display: inline-flex; align-items: center; gap: var(--sp-050); padding: var(--sp-050) var(--sp-100);
    border: 0; border-radius: var(--r-sm); background: transparent; color: inherit; font: inherit; cursor: pointer;
  }
  .switch:hover { background: var(--bg-surface-hover); }
  .quiet { color: var(--text-secondary); font-size: var(--fs-075); }
  .tabs { display: flex; gap: var(--sp-200); }
  .tab {
    padding-block: var(--sp-200); color: var(--text-secondary); text-decoration: none; white-space: nowrap;
    border-block-end: var(--bw-thick) solid transparent;
  }
  .tab:hover { color: var(--text-primary); }
  .group-label {
    margin: var(--sp-150) var(--sp-050) var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }
  .rows { margin: 0; padding: 0; list-style: none; }
  .tab[aria-current='page'] { color: var(--text-primary); font-weight: var(--fw-semibold); border-block-end-color: var(--accent-primary); }
</style>
