<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The *Templates* gallery (configuration.md §6, ui.md §4): every template the reader may
  // take, by its four origins, with its version, the version the workspace took and *Update
  // available*; taking or updating opens its `TemplateDialog`, the plan first. In a
  // hub's settings, the templates with a hub part: the hub takes that part for itself, once the
  // workspace took the template (configuration.md §6). In a collection's settings, the templates
  // with a collection part, which the collection takes under its own handle.

  import { untrack } from 'svelte';
  import { Button, DataTable, Dialog, Spinner, ViewSwitcher } from '@hubtask/design-system/components';
  import type { DataColumn } from '@hubtask/design-system/components';
  import TableSection from '../page/TableSection.svelte';
  import { Continued } from '../page/continued.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import type { ConfigTemplate, ConfigTemplateList, ResourceState } from '@hubtask/sync-engine';

  import TemplateDialog from './TemplateDialog.svelte';
  import TemplatePreview from './TemplatePreview.svelte';
  import { config } from '../data/config.svelte.ts';
  import { holds, holdsHere } from '../data/capability.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { byOrigin, galleryScope, offerOf } from '../data/configtemplates.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { familiesOf, familyOf, switchedOn } from '../workspace/hubGallery.ts';

  interface Props {
    /** A hub taking a template's hub part; omitted is the workspace. */
    hubId?: string;
    /** A collection taking a template's collection part; its hub follows from it. */
    collectionId?: string;
    /** A template to open the plan of at once, as a product's first visit chose it (§4). */
    preselect?: string;
    /** Where a taken template's main product home is opened (§3.2); without it the page stays. */
    onnavigate?: (path: string) => void;
  }

  const { hubId, collectionId, preselect, onnavigate }: Props = $props();
  /** Which gallery this is: its heading, its words and what it says when it offers nothing. */
  const scope = $derived(galleryScope(hubId, collectionId));

  let list = $state<ResourceState<ConfigTemplateList>>();
  $effect(() => {
    const hub = hubId;
    const collection = collectionId;
    return untrack(() => config.templates((next) => (list = next), hub, collection));
  });

  // A place's template the workspace has to take first, for a reader who may not change the
  // workspace: the card names who may, before the choice (user-experience.md §5.2) - never a
  // refusal after it. The names are the workspace's memberships, where the reader may read them.
  // Unknown while the manifest is read: nobody is sent to an administrator on a guess.
  const mayNotChangeWorkspace = $derived(holdsHere('STRUCTURE') === false);
  $effect(() => {
    if (scope === 'workspace' || !mayNotChangeWorkspace) return;
    return untrack(() => people.open({}));
  });
  const administratorNames = $derived.by(() => {
    if (scope === 'workspace' || !mayNotChangeWorkspace) return '';
    const names = people.administrators((role) => holds(role, 'STRUCTURE').status === 'permitted')
      .map((id) => accounts.nameOf(id)).filter((name): name is string => !!name);
    return names.length === 0 ? '' : new Intl.ListFormat(messages.locale, { type: 'conjunction' }).format(names);
  });
  const isAskedOf = (template: ConfigTemplate): boolean =>
    scope !== 'workspace' && mayNotChangeWorkspace && template.workspace_first === true && offerOf(template) === 'TAKE';

  // The family filter (user-experience.md §5.2): All, then each family the catalogue holds.
  let family = $state('all');
  const families = $derived(list?.status === 'ready' ? familiesOf(list.data.data) : []);
  const filterViews = $derived([
    { id: 'all', label: t('app.templates.family_all') },
    ...families.map((each) => ({ id: each, label: t(`app.workspace.hub_family.${each}`) })),
  ]);
  const shown = $derived(
    list?.status !== 'ready' ? [] : family === 'all' ? list.data.data : list.data.data.filter((each) => familyOf(each) === family),
  );
  const groups = $derived(byOrigin(shown));
  // A long catalogue continues at its foot (PS-8), at the device's rows per page; on a place's
  // settings, under the preset picker, a shorter first page keeps the section within two screens.
  // svelte-ignore state_referenced_locally
  const continued = new Continued(() => groups.flatMap((group) => group.templates), { first: hubId || collectionId ? 10 : undefined });
  $effect(() => {
    void family;
    untrack(() => continued.restart());
  });
  const refusal = $derived(list?.status === 'failed' ? renderProblem(list.error, messages) : undefined);

  /** Hubtask's templates are named by their key, in the reader's language. */
  const nameOf = (template: ConfigTemplate) =>
    template.origin === 'HUBTASK' ? t(`app.structure.preset.${template.key.toUpperCase()}`) : template.name;
  /** What a card says it does: Hubtask's own sentence, with the standard it follows; nothing for another's. */
  const summaryOf = (template: ConfigTemplate) =>
    template.origin === 'HUBTASK' ? t(`app.templates.summary.${template.key.toUpperCase()}`) : undefined;
  const productsOf = (template: ConfigTemplate): string[] =>
    switchedOn(template).map((module) => t(`app.modules.name_${module.toLowerCase()}`));

  /** The template opened in its dialog, and the one whose plan is being asked. */
  let opened = $state<ConfigTemplate>();
  /** The template whose content is shown, from the row's menu. */
  let previewed = $state<ConfigTemplate>();
  const columns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.settings.type_name'), priority: 1 },
    { id: 'products', label: t('app.templates.products'), priority: 3 },
    { id: 'state', label: t('app.templates.state'), priority: 2, isStatus: true },
  ]);
  let pending = $state<string>();

  // A template chosen on a product's empty home before its hub existed: its plan opens once the
  // list holds it, once. Unknown or no longer offered, the gallery is drawn as always.
  // Kept per key: the switch-on card links another template's plan onto this same screen.
  let preselected: string | undefined;
  $effect(() => {
    const key = preselect;
    if (!key || preselected === key || list?.status !== 'ready') return;
    const template = list.data.data.find((each) => each.key === key);
    preselected = key;
    if (template) untrack(() => (opened = template));
  });
</script>

<!-- A place's templates as one table (page-system §9.6, PS-4): the take or the update the row's
     verb, what it creates behind its ⋯. -->
<TableSection
  title={t(scope === 'collection' ? 'app.templates.collection_gallery' : scope === 'hub' ? 'app.templates.hub_gallery' : 'app.templates.gallery')}
  description={t(scope === 'collection' ? 'app.templates.collection_about' : scope === 'hub' ? 'app.templates.hub_about' : 'app.templates.about')}
>
  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if list?.status !== 'ready'}
    <Spinner label={t('app.templates.reading')} />
  {:else if scope === 'collection' && groups.length === 0}
    <p class="quiet" data-no-collection-templates>{t('app.templates.collection_none')}</p>
  {:else if hubId && groups.length === 0}
    <p class="quiet" data-no-hub-templates>{t('app.templates.hub_none')}</p>
  {:else}
    {#if families.length > 1}
      <ViewSwitcher label={t('app.templates.families')} views={filterViews} selected={family} onselect={(id) => (family = id)} />
    {/if}
    <div data-templates>
      <DataTable
        label={t('app.templates.gallery')}
        {columns}
        rows={continued.shown}
        shownLabel={continued.shownLabel}
        more={continued.more}
        perPage={continued.perPage}
        key={(template) => template.key}
        rowLabel={nameOf}
        group={(template) => t(`app.templates.origin.${template.origin}`)}
        menu={(template) => ({ label: t('app.page.more_for', { name: nameOf(template) }), items: [{ id: 'preview', label: t('app.templates.preview') }] })}
        onmenu={(template) => (previewed = template)}
        actionsLabel={t('app.page.row_actions')}
        sheetDismissLabel={t('app.page.sheet_close')}
        sheetGrabLabel={t('app.page.sheet_grab')}
      >
        {#snippet cell(template, column)}
          {@const offer = offerOf(template)}
          {@const isAsked = isAskedOf(template)}
          {#if column.id === 'name'}
            <span class="name" data-template={template.key} data-origin={template.origin} data-ask-admin={isAsked ? '' : undefined}>{nameOf(template)}</span>
          {:else if column.id === 'products'}
            <span class="quiet">{productsOf(template).join(', ')}</span>
          {:else if column.id === 'state'}
            <span class={offer === 'UPDATE' ? 'news' : 'quiet'}>
              {isAsked
                ? administratorNames ? t('app.templates.ask_admin_named', { names: administratorNames }) : t('app.templates.ask_admin')
                : offer === 'UPDATE'
                  ? t('app.templates.update_available', { from: template.taken_version ?? 0, to: template.version })
                  : offer === 'IN_USE'
                    ? t('app.templates.in_use', { version: template.version })
                    : ''}
            </span>
          {/if}
        {/snippet}
        {#snippet verb(template)}
          {@const offer = offerOf(template)}
          {#if offer !== 'IN_USE' && !isAskedOf(template)}
            <Button size="sm" tone={offer === 'UPDATE' ? 'primary' : 'secondary'} isBusy={pending === template.key}
              data-take={template.key} data-update-available={offer === 'UPDATE' ? '' : undefined} onclick={() => (opened = template)}>
              {t(offer === 'UPDATE' ? 'app.templates.update' : 'app.templates.take')}
            </Button>
          {/if}
        {/snippet}
      </DataTable>
    </div>
  {/if}
</TableSection>

<TemplateDialog template={opened} {hubId} {collectionId} {onnavigate} bind:pending onClose={() => (opened = undefined)} />

{#if previewed}
  <Dialog size="m" isOpen title={nameOf(previewed)} dismissLabel={t('app.page.close')} onClose={() => (previewed = undefined)}>
    {#if summaryOf(previewed)}<p class="quiet">{summaryOf(previewed)}</p>{/if}
    <TemplatePreview template={previewed} />
  </Dialog>
{/if}

<style>
  .name { font-weight: var(--fw-medium); }
  .news { color: var(--text-brand); font-weight: var(--fw-medium); }
  .quiet { margin: 0; color: var(--text-secondary); max-width: 64ch; }
</style>
