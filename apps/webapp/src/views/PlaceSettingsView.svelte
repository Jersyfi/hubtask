<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's or a collection's settings (UC-WRK-23, page-system §9.6): a settings page whose section
  // list takes the place of the place's tabs (PS-2), the place in the trail. Drawn only for a
  // reader who may change the shape (P-05); the sections, the shared draft and the save are
  // `StructurePanel`'s.

  import { untrack } from 'svelte';

  import { EmptyState, ReadOnlyNotice, Skeleton } from '@hubtask/design-system/components';
  import type { PageHead } from '@hubtask/design-system/components';

  import MembersDialog from '../lib/people/MembersDialog.svelte';
  import LazyView from '../lib/frame/LazyView.svelte';
  import { settingsParts } from '../lib/frame/parts.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { archivalOf } from '../lib/data/containers.ts';
  import { live } from '../lib/data/live.svelte.ts';
  import { modules, modulesHere } from '../lib/data/modules.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import TemplatesDialog from '../lib/entries/TemplatesDialog.svelte';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import StructurePanel from '../lib/workspace/StructurePanel.svelte';
  import PlaceGeneral from '../lib/workspace/PlaceGeneral.svelte';

  interface Props {
    id: string;
    /** The open section (`/…/:id/settings/:section`); none is the list. */
    section?: string;
    /** A template whose plan opens on the hub's *Templates and presets* (`?template=`). */
    template?: string;
    onnavigate: (path: string) => void;
    /** A move between sections replaces the address, so the shared draft stays. */
    onreplace: (path: string) => void;
  }

  const { id, section, template, onnavigate, onreplace }: Props = $props();

  $effect(() => {
    const wanted = id;
    return untrack(() => containers.openSingle(wanted));
  });
  const container = $derived(containers.find(id));
  const isHub = $derived(container?.type === 'HUB');
  $effect(() => {
    const parent = container?.type === 'COLLECTION' ? container.parent_id : undefined;
    if (!parent) return;
    return untrack(() => containers.openSingle(parent));
  });
  const hub = $derived(container?.type === 'COLLECTION' && container.parent_id ? containers.find(container.parent_id) : undefined);

  let placeModules = $state<readonly string[] | undefined>(undefined);
  $effect(() => {
    const at = id;
    placeModules = undefined;
    if (!at) return;
    return untrack(() => modules.here(at, (next) => {
      if (next.status === 'ready') placeModules = next.data.modules_on;
    }));
  });
  const modulesOn = $derived(modulesHere(manifest.value?.modules_on, placeModules));

  $effect(() => {
    if (container?.type !== 'HUB') return;
    const wanted = id;
    return untrack(() => containers.openLevel(wanted));
  });
  const collections = $derived(container?.type === 'HUB' ? containers.collectionsOf(id) : []);

  /** The path this place sits on, and the role the reader holds along it (`people.roleOf`). */
  const containerPath = $derived(
    container?.type === 'HUB' ? { hubId: container.id } : { hubId: container?.parent_id ?? undefined, collectionId: container?.id },
  );
  $effect(() => {
    if (!container) return;
    people.open(containerPath);
  });
  const structureRole = $derived((container ? people.roleOf(containerPath, actor.account?.id, roleOrder()) : undefined) ?? undefined);
  const mayShape = $derived(holds(structureRole, 'STRUCTURE').status === 'permitted');
  const mayShapeWorkspace = $derived(holds(people.roleOf({}, actor.account?.id, roleOrder()) ?? undefined, 'STRUCTURE').status === 'permitted');
  const memberScope = $derived(
    container?.type === 'HUB' ? { scopeType: 'HUB' as const, scopeId: container.id } : { scopeType: 'COLLECTION' as const, scopeId: container?.id ?? '' },
  );
  const memberCount = $derived(new Set(people.along(containerPath).map((membership) => membership.account_id)).size);
  const peopleStatus = $derived(memberCount > 0 ? t('app.settings.status_people', { count: memberCount }) : undefined);

  const archival = $derived(container ? archivalOf(container) : 'active');
  const readOnlyReason = $derived(
    archival === 'inherited'
      ? t('app.workspace.archived_above', { hub: hub?.name ?? '' })
      : archival === 'archived'
        ? t(mayShape ? 'app.workspace.archived' : 'app.workspace.archived_ask')
        : undefined,
  );

  const placePath = $derived(`/${isHub ? 'hubs' : 'collections'}/${id}`);
  const settingsPath = $derived(`${placePath}/settings`);
  const title = $derived(t('app.settings.title'));
  $effect(() => page.entitle(container ? t('app.settings.open', { name: container.name }) : undefined));

  const head = $derived<PageHead>({
    title,
    breadcrumb: {
      trail: [
        ...(hub ? [{ id: hub.id, label: hub.name, href: `/hubs/${hub.id}` }] : []),
        ...(container ? [{ id: container.id, label: container.name, href: placePath }] : []),
        { id: 'settings', label: title },
      ],
      label: t('app.workspace.trail'),
      expandLabel: t('app.workspace.expand_trail'),
    },
  });
</script>

{#if live.hasLost(id)}
  <EmptyState kind="filtered" title={t('app.live.access_revoked')} />
{:else if !container && !containers.isSettled(id)}
  <div aria-busy="true"><Skeleton lines={3} /></div>
{:else if !container}
  <EmptyState kind="filtered" title={t('app.workspace.not_loaded')} />
{:else if !mayShape || !container.structure}
  <EmptyState kind="filtered" title={t('app.settings.not_yours')} />
{:else if readOnlyReason}
  <ReadOnlyNotice text={readOnlyReason} />
{:else}
  {#snippet generalTab()}
    <PlaceGeneral {container} />
  {/snippet}
  {#snippet labelsTab()}
    <LazyView load={settingsParts.LabelsDialog} props={{ isEmbedded: true, isOpen: true, collectionId: container.id }} />
  {/snippet}
  {#snippet fieldsTab()}
    <LazyView load={settingsParts.CustomFieldsDialog} props={{ isEmbedded: true, isOpen: true, collectionId: container.id, role: structureRole }} />
  {/snippet}
  {#snippet templatesTab()}
    <TemplatesDialog isEmbedded isOpen collectionId={container.id} path={containerPath} role={structureRole} onopened={(itemId) => onnavigate(`/items/${itemId}`)} />
  {/snippet}
  {#snippet presetsTab()}
    <!-- The configuration templates this place takes (configuration.md §6): a catalogue beside the
         preset, never under the module switches (page-system §4.2 rule 4). -->
    {#if isHub}
      <LazyView load={settingsParts.SampleEntries} props={{ hubId: container.id }} />
      <LazyView load={settingsParts.TemplatesGallery} props={{ hubId: container.id, preselect: template, onnavigate }} />
    {:else}
      <LazyView load={settingsParts.TemplatesGallery} props={{ collectionId: container.id }} />
    {/if}
  {/snippet}
  {#snippet automationTab()}
    <LazyView load={settingsParts.PoliciesDialog} props={{ isEmbedded: true, isOpen: true, container, path: containerPath, role: structureRole }} />
  {/snippet}
  {#snippet modulesTab()}
    <LazyView load={settingsParts.ModuleSwitches} props={{ hubId: container.id, onnavigate }} />
  {/snippet}
  {#snippet calendarsTab()}
    <LazyView load={settingsParts.HubCalendars} props={{ hubId: container.id, mayShape }} />
    <LazyView load={settingsParts.HubSlaPolicies} props={{ hubId: container.id, mayShape }} />
  {/snippet}
  {#snippet codeTab()}
    <LazyView load={settingsParts.CodeHooks} props={{ hubId: container.id }} />
  {/snippet}
  {#snippet chatTab()}
    <LazyView load={settingsParts.ChatBots} props={{ hubId: container.id, collections }} />
  {/snippet}
  {#snippet peopleTab()}
    <MembersDialog isEmbedded isOpen title={t('app.people.title')} scope={memberScope} path={containerPath} />
  {/snippet}
  <StructurePanel
    isOpen
    {head}
    tab={section}
    {container}
    {mayShapeWorkspace}
    mayShapeHere={mayShape}
    hrefOf={(each: string) => `${settingsPath}/${each}`}
    onsection={(each: string) => onreplace(`${settingsPath}/${each}`)}
    onlist={() => onreplace(settingsPath)}
    before={[{ id: 'general', label: t('app.settings.section_general'), body: generalTab }]}
    tabs={container.type === 'COLLECTION'
      ? [
          { id: 'template', label: t('app.settings.section_template'), body: presetsTab },
          { id: 'labels', label: t('app.structure.tab_labels'), body: labelsTab },
          { id: 'fields', label: t('app.structure.tab_fields'), body: fieldsTab },
          { id: 'templates', label: t('app.settings.section_entry_templates'), body: templatesTab },
          { id: 'rules', label: t('app.settings.section_rules'), body: automationTab },
          { id: 'people', label: t('app.structure.tab_people'), body: peopleTab, status: peopleStatus },
        ]
      : [
          { id: 'modules', label: t('app.settings.section_products'), body: modulesTab, status: t('app.settings.status_on', { count: modulesOn.length }) },
          { id: 'template', label: t('app.settings.section_template'), body: presetsTab },
          // A hub's own working hours, where the service desk is on (UC-SVC-01 check 5).
          ...(modulesOn.includes('SERVICE_DESK') ? [{ id: 'calendars', label: t('app.desk.hub_section'), body: calendarsTab }] : []),
          // The forges that post its code (UC-INT-13 check 1).
          { id: 'code', label: t('app.code.section'), body: codeTab },
          // The Matrix bots in its rooms (UC-INT-18 check 1).
          { id: 'chat', label: t('app.chatbots.section'), body: chatTab },
          { id: 'people', label: t('app.structure.tab_people'), body: peopleTab, status: peopleStatus },
        ]}
  />
{/if}
