<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import BulkBar from './BulkBar.svelte';
  import Button from './Button.svelte';
  import Dialog from './Dialog.svelte';
  import EmptyState from './EmptyState.svelte';
  import FilterBar, { type Filter } from './FilterBar.svelte';
  import Checkbox from './Checkbox.svelte';
  import Input from './Input.svelte';
  import ListPage from './ListPage.svelte';
  import PlaceTabs from './PlaceTabs.svelte';
  import ResourceCard from './ResourceCard.svelte';
  import ResourceGrid from './ResourceGrid.svelte';
  import Select from './Select.svelte';
  import PeopleTable from './_PeopleTable.svelte';
  import Refused from './_RefusedDemo.svelte';
  import { templates } from './_pageFixtures.ts';

  const { content = 'full', list = 'people' }: { content?: 'full' | 'empty' | 'refused' | 'selecting'; list?: 'people' | 'templates' } = $props();

  let isAdding = $state(false);
  let isSelecting = $state(false);
  let selected = $state<string[]>([]);
  const selecting = $derived(isSelecting || content === 'selecting');
  const crumbs = $derived({ trail: [{ id: 'admin', label: 'Administration', href: '#admin' }, { id: 'people', label: list === 'templates' ? 'Templates' : 'People' }], label: 'Where you are', expandLabel: 'Show every level' });

  const peopleFilters: Filter[] = [
    { id: 'role', label: 'Role: any', isActive: true },
    { id: 'standing', label: 'Standing: active' },
    { id: 'group', label: 'Group' },
    { id: 'seen', label: 'Last seen' },
  ];
  const families: Filter[] = [
    { id: 'family', label: 'Family: all' },
    { id: 'use', label: 'In use' },
  ];
</script>

{#if list === 'templates'}
  <ListPage
    head={{ title: 'Templates', subtitle: content === 'empty' ? 'None saved yet' : '40 in the catalogue · 4 in use · 2 updates waiting', breadcrumb: crumbs, primary: { label: 'Save from a hub', icon: 'plus', onclick: () => {} } }}
    refused={content === 'refused' ? refused : undefined}
  >
    {#snippet strip()}<PlaceTabs label="Templates" current="catalogue" tabs={[{ id: 'catalogue', label: 'Catalogue', href: '#catalogue', count: 40 }, { id: 'use', label: 'In use', href: '#use', count: 4 }, { id: 'presets', label: 'Presets', href: '#presets', count: 0 }]} />{/snippet}
    {#snippet filters()}
      <FilterBar searchLabel="Search templates" clearSearchLabel="Clear the search" filters={families} filtersLabel="Filters" sheetTitle="Filters" sheetDismissLabel="Close" sheetGrabLabel="Pull to full height" showLabel="Show 40 templates" moreLabel="More filters" countLabel={content === 'empty' ? '0' : '40'}>
        {#snippet picker()}<Checkbox label="Teams" />{/snippet}
      </FilterBar>
    {/snippet}
    <ResourceGrid label="Templates" isEmpty={content === 'empty'}>
      {#each templates as template (template.id)}
        <ResourceCard title={template.title} description={template.description} mark={template.mark} chips={template.chips} state={'state' in template ? template.state : undefined} href={`#template-${template.id}`} />
      {/each}
      {#snippet empty()}
        <EmptyState kind="unused" title="No template saved yet" description="Save a hub as a template, and every new hub can start from it." icon="layout-template">
          {#snippet action()}<Button tone="primary" icon="plus">Save from a hub</Button>{/snippet}
        </EmptyState>
      {/snippet}
    </ResourceGrid>
  </ListPage>
{:else}
  <ListPage
    head={{
      title: 'People',
      subtitle: content === 'empty' ? 'Nobody yet' : '9 people · 3 groups · 12 roles',
      breadcrumb: crumbs,
      primary: { label: 'Add people', icon: 'plus', onclick: () => (isAdding = true) },
      secondary: [{ label: 'Import', onclick: () => {} }],
      menu: { label: 'More for People', items: [{ id: 'select', label: 'Select people' }, { id: 'export', label: 'Export the list' }, { id: 'removed', label: 'Show removed people' }], onselect: (id) => { if (id === 'select') isSelecting = true; } },
    }}
    bulk={selecting ? bulk : undefined}
    refused={content === 'refused' ? refused : undefined}
  >
    {#snippet strip()}<PlaceTabs label="People and access" current="people" tabs={[{ id: 'people', label: 'People', href: '#people', count: 9 }, { id: 'groups', label: 'Groups', href: '#groups', count: 3 }, { id: 'roles', label: 'Roles', href: '#roles', count: 12 }, { id: 'who', label: 'Who may do what', href: '#who' }]} />{/snippet}
    {#snippet filters()}
      <FilterBar searchLabel="Search people" clearSearchLabel="Clear the search" filters={peopleFilters} moreLabel="More filters" filtersLabel="Filters (1)" sheetTitle="Filters" sheetDismissLabel="Close" sheetGrabLabel="Pull to full height" showLabel="Show 9 people" clearLabel="Clear" countLabel={content === 'empty' ? '0 of 0' : '9 of 9'}>
        {#snippet picker()}<Checkbox label="Administrator" /><Checkbox label="Member" />{/snippet}
      </FilterBar>
    {/snippet}
    <PeopleTable content={content === 'empty' ? 'empty' : 'full'} isSelecting={selecting} bind:selected />
  </ListPage>
{/if}

<Dialog bind:isOpen={isAdding} size="m" title="Add people" dismissLabel="Close" primary={{ label: 'Send invitations', onclick: () => (isAdding = false) }}>
  <Input label="E-mail addresses" isRequired hint="Several at once, separated by commas." />
  <Select label="Role in the workspace" options={[{ value: 'member', label: 'Member' }]} value="member" />
  {#snippet actions()}<Button tone="subtle" onclick={() => (isAdding = false)}>Cancel</Button>{/snippet}
</Dialog>

{#snippet bulk()}
  <BulkBar label={`${selected.length} selected`} actions={[{ id: 'role', label: 'Change role…' }]} menu={{ label: 'More for the selected', items: [{ id: 'revoke', label: 'Revoke access…', isDestructive: true }] }} doneLabel="Done" onDone={() => { isSelecting = false; selected = []; }} />
{/snippet}

{#snippet refused()}<Refused what="the people of this workspace" />{/snippet}
