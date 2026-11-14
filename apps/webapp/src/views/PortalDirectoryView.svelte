<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Anfragen & Hilfe* (scene `portaldir`, PO-1, UC-SVC-14): every department one may ask - the
  // portals the server names for the account -, one search over their kinds of request and their
  // knowledge, and the reader's requests everywhere, newest first. Reached by *Erstellen*, the
  // search and *all departments* in every portal; the sidebar stays, this is a page of the work.

  import { untrack } from 'svelte';
  import { Icon, PageHeader, SearchField, Skeleton, Stack, WorkPage, type IconName } from '@hubtask/design-system/components';
  import { ICON_NAMES } from '@hubtask/design-system/icons';
  import type { PortalRequestPage, RequestCatalogue } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { cataloguePath, myRequestsPath } from '../lib/data/portal.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import DirectoryArticles from '../lib/portal/DirectoryArticles.svelte';
  import RequestDialog from '../lib/portal/RequestDialog.svelte';
  import RequestRows from '../lib/portal/RequestRows.svelte';
  import { newestEverywhere, wordsFind } from '../lib/portal/portal.ts';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';
  import ContainerMark from '../lib/workspace/ContainerMark.svelte';

  const { onnavigate }: { onnavigate?: (path: string) => void } = $props();
  $effect(() => page.entitle(t('app.portal_dir.title')));

  // At most the server's own bound of portals; each asked once for its kinds and the reader's requests.
  const portals = $derived(actor.portals);
  type Asked = { collectionId: string; name: string; catalogue: ReturnType<typeof resource<RequestCatalogue>>; mine: ReturnType<typeof resource<PortalRequestPage>> };
  let asked = $state<Asked[]>([]);
  $effect(() => {
    const now = portals;
    untrack(() => {
      asked = now.map((each) => asked.find((held) => held.collectionId === each.collection_id) ?? {
        collectionId: each.collection_id, name: each.name,
        catalogue: resource<RequestCatalogue>(cataloguePath(each.collection_id)),
        mine: resource<PortalRequestPage>(myRequestsPath(each.collection_id)),
      });
    });
  });
  const isLoading = $derived(actor.account === undefined || asked.some((each) => each.catalogue.state.status === 'loading'));
  const departments = $derived(asked.map((each) => {
    const answered = each.catalogue.state.status === 'ready' ? each.catalogue.state.data : undefined;
    return { ...each, desk: answered?.desk, kinds: answered && answered.may_raise !== false ? answered.data : [] };
  }));
  const everywhere = $derived(newestEverywhere(asked.map((each) => (each.mine.state.status === 'ready' ? each.mine.state.data.data : [])), 8));
  const nameOf = (collectionId: string) => asked.find((each) => each.collectionId === collectionId)?.name;

  let query = $state('');
  let settled = $state('');
  $effect(() => {
    const words = query.trim();
    const timer = setTimeout(() => (settled = words), 300);
    return () => clearTimeout(timer);
  });
  const foundKinds = $derived(query.trim() === '' ? [] : departments.flatMap((department) =>
    department.kinds.filter((kind) => wordsFind(query, kind.name, kind.key, kind.description, department.name)).map((kind) => ({ department, kind }))).slice(0, 8));
  const markOf = (icon: string | undefined | null): IconName => ((ICON_NAMES as readonly string[]).includes(icon ?? '') ? (icon as IconName) : 'inbox');

  let asking = $state<{ collectionId: string; kind: (typeof departments)[number]['kinds'][number]; words: string } | undefined>(undefined);
  const go = (path: string) => (event: MouseEvent) => {
    if (!onnavigate) return;
    event.preventDefault();
    onnavigate(path);
  };
</script>

<WorkPage>
  {#snippet top()}
    <!-- Scene `portaldir`: the place as the trail, the question as the title. -->
    <PageHeader title={viewport.isCompact ? t('app.portal_dir.title') : t('app.portal_dir.heading')} subtitle={t('app.portal_dir.intro')} isTitleInBar={viewport.isCompact}
      breadcrumb={viewport.isCompact ? undefined : { trail: [{ id: 'portals', label: t('app.portal_dir.title') }], label: t('app.portal_dir.trail'), expandLabel: t('app.admin.expand_trail') }} />
  {/snippet}
  <div class="dir" data-portal-directory>
    {#if portals.length === 0 && actor.account !== undefined}
      <ProductEmpty product="SERVICE_DESK" kind="unused" title={t('app.portal_dir.none')} description={t('app.portal_dir.none_body')} />
    {:else}
      <div class="search" role="search">
        <SearchField label={t('app.portal_dir.search')} isLabelHidden clearLabel={t('app.portal.search_clear')} size="md"
          placeholder={t('app.portal_dir.search_placeholder')} bind:value={query} data-portal-dir-search />
        {#if query.trim() !== ''}
          <div class="found" data-portal-dir-found>
            {#if foundKinds.length > 0}
              <p class="group">{t('app.portal_dir.found_kinds')}</p>
              {#each foundKinds as { department, kind } (`${department.collectionId}/${kind.key}`)}
                <button type="button" class="hit" data-portal-dir-kind={kind.key}
                  onclick={() => (asking = { collectionId: department.collectionId, kind, words: query.trim() })}>
                  <Icon name={markOf(kind.icon)} size="sm" /><span>{kind.name || kind.key}</span><span class="where">· {department.name}</span>
                </button>
              {/each}
            {/if}
            {#key settled}
              <DirectoryArticles portals={portals.slice(0, 12)} text={settled} onopen={(path) => onnavigate?.(path)} />
            {/key}
          </div>
        {/if}
      </div>

      <section aria-labelledby="dir-departments">
        <h2 class="label" id="dir-departments">{t('app.portal_dir.departments')}</h2>
        {#if isLoading && departments.length === 0}
          <div aria-busy="true"><Skeleton lines={2} /></div>
        {:else}
          <ul class="tiles">
            {#each departments as department (department.collectionId)}
              <li>
                <a class="tile" href={`/portal/${department.collectionId}`} data-portal-department={department.collectionId} onclick={go(`/portal/${department.collectionId}`)}>
                  <ContainerMark look={{ type: 'COLLECTION', icon: department.desk?.icon || null, color_token: department.desk?.color_token || null }} size="sm" />
                  <span class="words">
                    <span class="name">{department.desk?.name || department.name}</span>
                    {#if department.desk?.description}<span class="sub">{department.desk.description}</span>
                    {:else if department.kinds.length > 0}<span class="sub">{department.kinds.slice(0, 3).map((kind) => kind.name || kind.key).join(', ')}</span>{/if}
                  </span>
                </a>
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <div class="pair">
        <section class="card" aria-labelledby="dir-mine" data-portal-dir-mine>
          <h2 class="heading" id="dir-mine">{t('app.portal_dir.mine')}</h2>
          {#if everywhere.length === 0}
            <ProductEmpty product="SERVICE_DESK" kind="settled" title={t('app.portal_dir.mine_none')} />
          {:else}
            <RequestRows requests={everywhere} departmentOf={nameOf} {onnavigate} />
          {/if}
        </section>
        <section class="card" aria-labelledby="dir-how">
          <h2 class="heading" id="dir-how">{t('app.portal_dir.how')}</h2>
          <Stack gap="050">
            <p class="quiet"><Icon name="plus" size="sm" /> {t('app.portal_dir.how_create')}</p>
            <p class="quiet"><Icon name="search" size="sm" /> {t('app.portal_dir.how_search')}</p>
            <p class="quiet"><Icon name="link" size="sm" /> {t('app.portal_dir.how_portal')}</p>
            <p class="quiet"><Icon name="arrow-left" size="sm" /> {t('app.portal_dir.how_back')}</p>
          </Stack>
        </section>
      </div>
    {/if}
  </div>
</WorkPage>

{#if asking}
  {#key `${asking.collectionId}/${asking.kind.key}`}
    <RequestDialog collectionId={asking.collectionId} kind={asking.kind} words={asking.words} onclose={() => (asking = undefined)} {onnavigate} />
  {/key}
{/if}

<style>
  .dir { display: flex; flex-direction: column; gap: var(--sp-300); container-type: inline-size; }
  .search { inline-size: min(100%, var(--layout-dialog-l)); }
  .found {
    margin-block-start: var(--sp-100); padding: var(--sp-150); display: flex; flex-direction: column; gap: var(--sp-050);
    background: var(--bg-raised); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); box-shadow: var(--shadow-overlay);
  }
  .group, .label { margin: 0; font-size: var(--fs-075); font-weight: var(--fw-semibold); color: var(--text-secondary); text-transform: uppercase; }
  .label { margin-block-end: var(--sp-100); }
  .hit { display: flex; align-items: center; gap: var(--sp-100); padding: var(--sp-100); border: 0; border-radius: var(--r-sm); background: transparent; color: inherit; font: inherit; text-align: start; cursor: pointer; }
  .hit:hover, .hit:focus-visible { background: var(--accent-primary-subtle); }
  .where, .sub, .quiet { color: var(--text-secondary); font-size: var(--fs-075); }
  .quiet { margin: 0; display: flex; gap: var(--sp-050); align-items: center; }
  .heading { margin: 0 0 var(--sp-100); font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .tiles { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--sp-200); grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--layout-gallery-card)), 1fr)); }
  .tile {
    box-sizing: border-box; block-size: 100%; display: flex; gap: var(--sp-150); align-items: flex-start; padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); color: inherit; text-decoration: none;
  }
  .tile:hover { background: var(--bg-surface-hover); }
  .tile:focus-visible { outline: var(--focus-ring); }
  .words { display: flex; flex-direction: column; gap: var(--sp-025); min-inline-size: 0; }
  .name { font-weight: var(--fw-semibold); }
  .pair { display: grid; gap: var(--sp-300); grid-template-columns: minmax(0, 1fr); }
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a container query cannot read a custom property. */
  @container (inline-size >= 905px) {
    .pair { grid-template-columns: minmax(0, 3fr) minmax(0, 2fr); }
  }
  .card { padding: var(--sp-200); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); }
</style>
